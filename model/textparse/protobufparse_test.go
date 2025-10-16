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
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
	"github.com/prometheus/prometheus/util/pool"
)

func metricFamiliesToProtobuf(t testing.TB, testMetricFamilies []string) *bytes.Buffer {
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

func createTestProtoBuf(t testing.TB) *bytes.Buffer {
	t.Helper()

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
		`name: "test_histogram3"
help: "Similar histogram as before but now with integer buckets."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 6
    sample_sum: 50
    bucket: <
      cumulative_count: 2
      upper_bound: -20
    >
    bucket: <
      cumulative_count: 4
      upper_bound: 20
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: 15
        timestamp: <
          seconds: 1625851153
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 6
      upper_bound: 30
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: 25
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
      seconds: 1625851153
      nanos: 146848499
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
      seconds: 1625851153
      nanos: 146848499
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
      seconds: 1625851153
      nanos: 146848499
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
      seconds: 1625851153
      nanos: 146848499
    >
    positive_span: <
      offset: 0
      length: 0
    >
  >
>

`,
		`name: "test_histogram_with_native_histogram_exemplars"
help: "A histogram with native histogram exemplars."
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
    exemplars: <
      label: <
        name: "dummyID"
        value: "59780"
      >
      value: -0.00039
      timestamp: <
        seconds: 1625851155
        nanos: 146848499
      >
    >
    exemplars: <
      label: <
        name: "dummyID"
        value: "5617"
      >
      value: -0.00029
    >
    exemplars: <
      label: <
        name: "dummyID"
        value: "59772"
      >
      value: -0.00052
      timestamp: <
        seconds: 1625851160
        nanos: 156848499
      >
    >
  >
  timestamp_ms: 1234568
>

`,

		`name: "test_histogram_with_native_histogram_exemplars2"
help: "Another histogram with native histogram exemplars."
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
    exemplars: <
      label: <
        name: "dummyID"
        value: "59780"
      >
      value: -0.00039
      timestamp: <
        seconds: 1625851155
        nanos: 146848499
      >
    >
  >
  timestamp_ms: 1234568
>

`,
	}

	return metricFamiliesToProtobuf(t, testMetricFamilies)
}

func TestProtobufParse(t *testing.T) {
	inputBuf := createTestProtoBuf(t)

	scenarios := []struct {
		name     string
		parser   Parser
		expected []parsedEntry
	}{
		{
			name:   "ignoreNativeHistograms=false/parseClassicHistograms=false/enableTypeAndUnitLabels=false",
			parser: NewProtobufParser(inputBuf.Bytes(), false, false, false, false, labels.NewSymbolTable()),
			expected: []parsedEntry{
				{
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
				},
				{
					m: "go_build_info\xffchecksum\xff\xffpath\xffgithub.com/prometheus/client_golang\xffversion\xff(devel)",
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
					m:    "go_memstats_alloc_bytes_total",
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234567),
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
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
					es: []exemplar.Exemplar{
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
					es: []exemplar.Exemplar{
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
					m:    "test_histogram3",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:   "test_histogram3",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram3_count",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_count",
					),
				},
				{
					m: "test_histogram3_sum",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_sum",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: false},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
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
					m: "rpc_durations_seconds\xffquantile\xff0.5\xffservice\xffexponential",
					v: 6.442786329648548e-07,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.5",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.9\xffservice\xffexponential",
					v: 1.9435742936658396e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.9",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.99\xffservice\xffexponential",
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
					ct: 1625851153146,
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
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_summary_with_createdtimestamp_sum",
					v:  1.234,
					ct: 1625851153146,
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
					ct: 1625851153146,
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
					ct: 1625851153146,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp",
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars",
					help: "A histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
						{Labels: labels.FromStrings("dummyID", "59772"), Value: -0.00052, HasTs: true, Ts: 1625851160156},
					},
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars2",
					help: "Another histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars2",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars2",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
			},
		},
		{
			name:   "ignoreNativeHistograms=false/parseClassicHistograms=false/enableTypeAndUnitLabels=true",
			parser: NewProtobufParser(inputBuf.Bytes(), false, false, false, true, labels.NewSymbolTable()),
			expected: []parsedEntry{
				{
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
				},
				{
					m: "go_build_info\xff__type__\xffgauge\xffchecksum\xff\xffpath\xffgithub.com/prometheus/client_golang\xffversion\xff(devel)",
					v: 1,
					lset: labels.FromStrings(
						"__name__", "go_build_info",
						"__type__", string(model.MetricTypeGauge),
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
					m:    "go_memstats_alloc_bytes_total",
					unit: "bytes",
				},
				{
					m:   "go_memstats_alloc_bytes_total",
					typ: model.MetricTypeCounter,
				},
				{
					m: "go_memstats_alloc_bytes_total\xff__type__\xffcounter\xff__unit__\xffbytes",
					v: 1.546544e+06,
					lset: labels.FromStrings(
						"__name__", "go_memstats_alloc_bytes_total",
						"__type__", string(model.MetricTypeCounter),
						"__unit__", "bytes",
					),
					es: []exemplar.Exemplar{
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
					t: int64p(1234567),
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
					m: "test_histogram\xff__type__\xffhistogram",
					t: int64p(1234568),
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
						"__type__", string(model.MetricTypeHistogram),
					),
					es: []exemplar.Exemplar{
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
					m: "test_gauge_histogram\xff__type__\xffgaugehistogram",
					t: int64p(1234568),
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
						"__type__", string(model.MetricTypeGaugeHistogram),
					),
					es: []exemplar.Exemplar{
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
					m: "test_float_histogram\xff__type__\xffhistogram",
					t: int64p(1234568),
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
						"__type__", string(model.MetricTypeHistogram),
					),
					es: []exemplar.Exemplar{
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
					m: "test_gauge_float_histogram\xff__type__\xffgaugehistogram",
					t: int64p(1234568),
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
						"__type__", string(model.MetricTypeGaugeHistogram),
					),
					es: []exemplar.Exemplar{
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
					m: "test_histogram2_count\xff__type__\xffhistogram",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_count",
						"__type__", string(model.MetricTypeHistogram),
					),
				},
				{
					m: "test_histogram2_sum\xff__type__\xffhistogram",
					v: 0.000828,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_sum",
						"__type__", string(model.MetricTypeHistogram),
					),
				},
				{
					m: "test_histogram2_bucket\xff__type__\xffhistogram\xffle\xff-0.00048",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "-0.00048",
					),
				},
				{
					m: "test_histogram2_bucket\xff__type__\xffhistogram\xffle\xff-0.00038",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "-0.00038",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_bucket\xff__type__\xffhistogram\xffle\xff1.0",
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "1.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.000295, HasTs: false},
					},
				},
				{
					m: "test_histogram2_bucket\xff__type__\xffhistogram\xffle\xff+Inf",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "+Inf",
					),
				},
				{
					m:    "test_histogram3",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:   "test_histogram3",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram3_count\xff__type__\xffhistogram",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_count",
						"__type__", string(model.MetricTypeHistogram),
					),
				},
				{
					m: "test_histogram3_sum\xff__type__\xffhistogram",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_sum",
						"__type__", string(model.MetricTypeHistogram),
					),
				},
				{
					m: "test_histogram3_bucket\xff__type__\xffhistogram\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram3_bucket\xff__type__\xffhistogram\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram3_bucket\xff__type__\xffhistogram\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"__type__", string(model.MetricTypeHistogram),
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: false},
					},
				},
				{
					m: "test_histogram3_bucket\xff__type__\xffhistogram\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"__type__", string(model.MetricTypeHistogram),
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
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbar",
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
						"__type__", string(model.MetricTypeHistogram),
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbaz",
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
						"__type__", string(model.MetricTypeHistogram),
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
					m: "test_float_histogram_with_zerothreshold_zero\xff__type__\xffhistogram",
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
						"__type__", string(model.MetricTypeHistogram),
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
					m: "rpc_durations_seconds_count\xff__type__\xffsummary\xffservice\xffexponential",
					v: 262,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds_count",
						"__type__", string(model.MetricTypeSummary),
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds_sum\xff__type__\xffsummary\xffservice\xffexponential",
					v: 0.00025551262820703587,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds_sum",
						"__type__", string(model.MetricTypeSummary),
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xff__type__\xffsummary\xffquantile\xff0.5\xffservice\xffexponential",
					v: 6.442786329648548e-07,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"__type__", string(model.MetricTypeSummary),
						"quantile", "0.5",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xff__type__\xffsummary\xffquantile\xff0.9\xffservice\xffexponential",
					v: 1.9435742936658396e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"__type__", string(model.MetricTypeSummary),
						"quantile", "0.9",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xff__type__\xffsummary\xffquantile\xff0.99\xffservice\xffexponential",
					v: 4.0471608667037015e-06,
					lset: labels.FromStrings(
						"__type__", string(model.MetricTypeSummary),
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
					m: "without_quantiles_count\xff__type__\xffsummary",
					v: 42,
					lset: labels.FromStrings(
						"__name__", "without_quantiles_count",
						"__type__", string(model.MetricTypeSummary),
					),
				},
				{
					m: "without_quantiles_sum\xff__type__\xffsummary",
					v: 1.234,
					lset: labels.FromStrings(
						"__name__", "without_quantiles_sum",
						"__type__", string(model.MetricTypeSummary),
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
					m: "empty_histogram\xff__type__\xffhistogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
						"__type__", string(model.MetricTypeHistogram),
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
					m:  "test_counter_with_createdtimestamp\xff__type__\xffcounter",
					v:  42,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_counter_with_createdtimestamp",
						"__type__", string(model.MetricTypeCounter),
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
					m:  "test_summary_with_createdtimestamp_count\xff__type__\xffsummary",
					v:  42,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
						"__type__", string(model.MetricTypeSummary),
					),
				},
				{
					m:  "test_summary_with_createdtimestamp_sum\xff__type__\xffsummary",
					v:  1.234,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_sum",
						"__type__", string(model.MetricTypeSummary),
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
					m:  "test_histogram_with_createdtimestamp\xff__type__\xffhistogram",
					ct: 1625851153146,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp",
						"__type__", string(model.MetricTypeHistogram),
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
					m:  "test_gaugehistogram_with_createdtimestamp\xff__type__\xffgaugehistogram",
					ct: 1625851153146,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp",
						"__type__", string(model.MetricTypeGaugeHistogram),
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars",
					help: "A histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars\xff__type__\xffhistogram",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars",
						"__type__", string(model.MetricTypeHistogram),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
						{Labels: labels.FromStrings("dummyID", "59772"), Value: -0.00052, HasTs: true, Ts: 1625851160156},
					},
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars2",
					help: "Another histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars2",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2\xff__type__\xffhistogram",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars2",
						"__type__", string(model.MetricTypeHistogram),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
			},
		},
		{
			name:   "ignoreNativeHistograms=false/parseClassicHistograms=true/enableTypeAndUnitLabels=false",
			parser: NewProtobufParser(inputBuf.Bytes(), false, true, false, false, labels.NewSymbolTable()),
			expected: []parsedEntry{
				{
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
				},
				{
					m: "go_build_info\xffchecksum\xff\xffpath\xffgithub.com/prometheus/client_golang\xffversion\xff(devel)",
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
					m:    "go_memstats_alloc_bytes_total",
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234567),
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_count",
					),
				},
				{
					m: "test_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_sum",
					),
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "+Inf",
					),
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_count",
					),
				},
				{
					m: "test_gauge_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_sum",
					),
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "+Inf",
					),
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_float_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_count",
					),
				},
				{
					m: "test_float_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_sum",
					),
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_float_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "+Inf",
					),
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
					t: int64p(1234568),
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
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_float_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_count",
					),
				},
				{
					m: "test_gauge_float_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_sum",
					),
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "+Inf",
					),
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
					es: []exemplar.Exemplar{
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
					es: []exemplar.Exemplar{
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
					m:    "test_histogram3",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:   "test_histogram3",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram3_count",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_count",
					),
				},
				{
					m: "test_histogram3_sum",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_sum",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: false},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
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
					m: "test_histogram_family_count\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "+Inf",
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
					m: "test_histogram_family_count\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff1.1",
					v: 1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "+Inf",
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
					m: "rpc_durations_seconds\xffquantile\xff0.5\xffservice\xffexponential",
					v: 6.442786329648548e-07,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.5",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.9\xffservice\xffexponential",
					v: 1.9435742936658396e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.9",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.99\xffservice\xffexponential",
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
					ct: 1625851153146,
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
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_summary_with_createdtimestamp_sum",
					v:  1.234,
					ct: 1625851153146,
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
					ct: 1625851153146,
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
					ct: 1625851153146,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp",
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars",
					help: "A histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
						{Labels: labels.FromStrings("dummyID", "59772"), Value: -0.00052, HasTs: true, Ts: 1625851160156},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_count",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_sum",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "+Inf",
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars2",
					help: "Another histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars2",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2",
					t: int64p(1234568),
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
						"__name__", "test_histogram_with_native_histogram_exemplars2",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59780"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_count",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_sum",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0003899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0002899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "+Inf",
					),
				},
			},
		},
		{
			name:   "ignoreNativeHistograms=true/parseClassicHistograms=false/enableTypeAndUnitLabels=false",
			parser: NewProtobufParser(inputBuf.Bytes(), true, false, false, false, labels.NewSymbolTable()),
			expected: []parsedEntry{
				{
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
				},
				{
					m: "go_build_info\xffchecksum\xff\xffpath\xffgithub.com/prometheus/client_golang\xffversion\xff(devel)",
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
					m:    "go_memstats_alloc_bytes_total",
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
					es: []exemplar.Exemplar{
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
					t: int64p(1234567),
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
					m: "test_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_count",
					),
				},
				{
					m: "test_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_sum",
					),
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "+Inf",
					),
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
					m: "test_gauge_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_count",
					),
				},
				{
					m: "test_gauge_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_sum",
					),
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_gauge_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "+Inf",
					),
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
					m: "test_float_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_count",
					),
				},
				{
					m: "test_float_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_sum",
					),
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_float_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "+Inf",
					),
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
					m: "test_gauge_float_histogram_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_count",
					),
				},
				{
					m: "test_gauge_float_histogram_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_sum",
					),
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_gauge_float_histogram_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "+Inf",
					),
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
					es: []exemplar.Exemplar{
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
					es: []exemplar.Exemplar{
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
					m:    "test_histogram3",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:   "test_histogram3",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram3_count",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_count",
					),
				},
				{
					m: "test_histogram3_sum",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_sum",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram3_bucket\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: false},
					},
				},
				{
					m: "test_histogram3_bucket\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram3_bucket",
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
					m: "test_histogram_family_count\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family_count\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff1.1",
					v: 1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "+Inf",
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
					m: "test_float_histogram_with_zerothreshold_zero_count",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_with_zerothreshold_zero_count",
					),
				},
				{
					m: "test_float_histogram_with_zerothreshold_zero_sum",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_with_zerothreshold_zero_sum",
					),
				},
				{
					m: "test_float_histogram_with_zerothreshold_zero_bucket\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_with_zerothreshold_zero_bucket",
						"le", "+Inf",
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
					m: "rpc_durations_seconds\xffquantile\xff0.5\xffservice\xffexponential",
					v: 6.442786329648548e-07,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.5",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.9\xffservice\xffexponential",
					v: 1.9435742936658396e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.9",
						"service", "exponential",
					),
				},
				{
					m: "rpc_durations_seconds\xffquantile\xff0.99\xffservice\xffexponential",
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
					m: "empty_histogram_count",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "empty_histogram_count",
					),
				},
				{
					m: "empty_histogram_sum",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "empty_histogram_sum",
					),
				},
				{
					m: "empty_histogram_bucket\xffle\xff+Inf",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "empty_histogram_bucket",
						"le", "+Inf",
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
					ct: 1625851153146,
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
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_summary_with_createdtimestamp_sum",
					v:  1.234,
					ct: 1625851153146,
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
					m:  "test_histogram_with_createdtimestamp_count",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_histogram_with_createdtimestamp_sum",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp_sum",
					),
				},
				{
					m:  "test_histogram_with_createdtimestamp_bucket\xffle\xff+Inf",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp_bucket",
						"le", "+Inf",
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
					m:  "test_gaugehistogram_with_createdtimestamp_count",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_gaugehistogram_with_createdtimestamp_sum",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp_sum",
					),
				},
				{
					m:  "test_gaugehistogram_with_createdtimestamp_bucket\xffle\xff+Inf",
					v:  0,
					ct: 1625851153146,
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp_bucket",
						"le", "+Inf",
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars",
					help: "A histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_count",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_sum",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0003899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "-0.0002899999999999998",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{
					m: "test_histogram_with_native_histogram_exemplars_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars_bucket",
						"le", "+Inf",
					),
				},
				{
					m:    "test_histogram_with_native_histogram_exemplars2",
					help: "Another histogram with native histogram exemplars.",
				},
				{
					m:   "test_histogram_with_native_histogram_exemplars2",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_count",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_count",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_sum",
					t: int64p(1234568),
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_sum",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0004899999999999998",
					t: int64p(1234568),
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0003899999999999998",
					t: int64p(1234568),
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0003899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff-0.0002899999999999998",
					t: int64p(1234568),
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "-0.0002899999999999998",
					),
				},
				{
					m: "test_histogram_with_native_histogram_exemplars2_bucket\xffle\xff+Inf",
					t: int64p(1234568),
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_native_histogram_exemplars2_bucket",
						"le", "+Inf",
					),
				},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var (
				p   = scenario.parser
				exp = scenario.expected
			)
			got := testParse(t, p)
			requireEntries(t, exp, got)
		})
	}
}

// TestProtobufParseWithNHCB is only concerned with classic histograms.
func TestProtobufParseWithNHCB(t *testing.T) {
	testMetricFamilies := []string{
		`name: "test_histogram1"
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
		`name: "test_histogram2_seconds"
help: "Similar histogram as before but now with integer buckets."
type: HISTOGRAM
unit: "seconds"
metric: <
  histogram: <
    sample_count: 6
    sample_sum: 50
    bucket: <
      cumulative_count: 2
      upper_bound: -20
    >
    bucket: <
      cumulative_count: 4
      upper_bound: 20
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: 15
        timestamp: <
          seconds: 1625851153
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 6
      upper_bound: 30
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: 25
		timestamp: <
          seconds: 1625851153
          nanos: 146848499
        >
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
      cumulative_count: 0
      upper_bound: 1.1
    >
    bucket: <
      cumulative_count: 5
      upper_bound: 2.2
    >
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
	}

	buf := metricFamiliesToProtobuf(t, testMetricFamilies)

	data := buf.Bytes()

	testCases := []struct {
		ignoreNative bool
		keepClassic  bool
		typeAndUnit  bool
		expected     []parsedEntry
	}{
		{
			ignoreNative: false,
			keepClassic:  false,
			typeAndUnit:  false,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
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
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
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
			},
		},
		{
			ignoreNative: false,
			keepClassic:  true,
			typeAndUnit:  false,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1_count",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_count",
					),
				},
				{
					m: "test_histogram1_sum",
					v: 0.000828,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_sum",
					),
				},
				{
					m: "test_histogram1_bucket\xffle\xff-0.00048",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "-0.00048",
					),
				},
				{
					m: "test_histogram1_bucket\xffle\xff-0.00038",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "-0.00038",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram1_bucket\xffle\xff1.0",
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "1.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.000295, HasTs: false},
					},
				},
				{
					m: "test_histogram1_bucket\xffle\xff+Inf",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram1",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds_count",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_count",
					),
				},
				{
					m: "test_histogram2_seconds_sum",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_sum",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram2_seconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
					m: "test_histogram_family_count\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_count\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff1.1",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
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
			},
		},
		{
			ignoreNative: true,
			keepClassic:  false,
			typeAndUnit:  false,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
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
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
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
						Schema:           -53,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
						CustomValues:     []float64{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
					),
				},
			},
		},
		{
			ignoreNative: true,
			keepClassic:  true,
			typeAndUnit:  false,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1_count",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_count",
					),
				},
				{
					m: "test_histogram1_sum",
					v: 0.000828,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_sum",
					),
				},
				{
					m: "test_histogram1_bucket\xffle\xff-0.00048",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "-0.00048",
					),
				},
				{
					m: "test_histogram1_bucket\xffle\xff-0.00038",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "-0.00038",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram1_bucket\xffle\xff1.0",
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "1.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.000295, HasTs: false},
					},
				},
				{
					m: "test_histogram1_bucket\xffle\xff+Inf",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram1",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds_count",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_count",
					),
				},
				{
					m: "test_histogram2_seconds_sum",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_sum",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram2_seconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
					m: "test_histogram_family_count\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_count\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_sum\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff1.1",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
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
					m: "empty_histogram_count",
					lset: labels.FromStrings(
						"__name__", "empty_histogram_count",
					),
				},
				{
					m: "empty_histogram_sum",
					lset: labels.FromStrings(
						"__name__", "empty_histogram_sum",
					),
				},
				{
					m: "empty_histogram_bucket\xffle\xff+Inf",
					lset: labels.FromStrings(
						"__name__", "empty_histogram_bucket",
						"le", "+Inf",
					),
				},
				{
					m: "empty_histogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Schema:           -53,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
					),
				},
			},
		},
		{
			ignoreNative: false,
			keepClassic:  false,
			typeAndUnit:  true,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1\xff__type__\xffhistogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
						"__type__", "histogram",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds\xff__type__\xffhistogram\xff__unit__\xffseconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
						"__type__", "histogram",
						"__unit__", "seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"__type__", "histogram",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"__type__", "histogram",
						"foo", "baz",
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
					m: "empty_histogram\xff__type__\xffhistogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
						"__type__", "histogram",
					),
				},
			},
		},
		{
			ignoreNative: false,
			keepClassic:  true,
			typeAndUnit:  true,
			expected: []parsedEntry{
				{
					m:    "test_histogram1",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram1",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram1_count\xff__type__\xffhistogram",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_count",
						"__type__", "histogram",
					),
				},
				{
					m: "test_histogram1_sum\xff__type__\xffhistogram",
					v: 0.000828,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_sum",
						"__type__", "histogram",
					),
				},
				{
					m: "test_histogram1_bucket\xff__type__\xffhistogram\xffle\xff-0.00048",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"__type__", "histogram",
						"le", "-0.00048",
					),
				},
				{
					m: "test_histogram1_bucket\xff__type__\xffhistogram\xffle\xff-0.00038",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"__type__", "histogram",
						"le", "-0.00038",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram1_bucket\xff__type__\xffhistogram\xffle\xff1.0",
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"__type__", "histogram",
						"le", "1.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.000295, HasTs: false},
					},
				},
				{
					m: "test_histogram1_bucket\xff__type__\xffhistogram\xffle\xff+Inf",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram1_bucket",
						"__type__", "histogram",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram1\xff__type__\xffhistogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            175,
						Sum:              0.000828,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 4},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 10, 147},
						CustomValues:    []float64{-0.00048, -0.00038, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram1",
						"__type__", "histogram",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
						// The second exemplar has no timestamp.
					},
				},
				{
					m:    "test_histogram2_seconds",
					help: "Similar histogram as before but now with integer buckets.",
				},
				{
					m:    "test_histogram2_seconds",
					unit: "seconds",
				},
				{
					m:   "test_histogram2_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram2_seconds_count\xff__type__\xffhistogram\xff__unit__\xffseconds",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_count",
						"__type__", "histogram",
						"__unit__", "seconds",
					),
				},
				{
					m: "test_histogram2_seconds_sum\xff__type__\xffhistogram\xff__unit__\xffseconds",
					v: 50,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_sum",
						"__type__", "histogram",
						"__unit__", "seconds",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xff__type__\xffhistogram\xff__unit__\xffseconds\xffle\xff-20.0",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"__type__", "histogram",
						"__unit__", "seconds",
						"le", "-20.0",
					),
				},
				{
					m: "test_histogram2_seconds_bucket\xff__type__\xffhistogram\xff__unit__\xffseconds\xffle\xff20.0",
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"__type__", "histogram",
						"__unit__", "seconds",
						"le", "20.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xff__type__\xffhistogram\xff__unit__\xffseconds\xffle\xff30.0",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"__type__", "histogram",
						"__unit__", "seconds",
						"le", "30.0",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
				},
				{
					m: "test_histogram2_seconds_bucket\xff__type__\xffhistogram\xff__unit__\xffseconds\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds_bucket",
						"__type__", "histogram",
						"__unit__", "seconds",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram2_seconds\xff__type__\xffhistogram\xff__unit__\xffseconds",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              50,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 0, 0},
						CustomValues:    []float64{-20, 20, 30},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram2_seconds",
						"__type__", "histogram",
						"__unit__", "seconds",
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: 15, HasTs: true, Ts: 1625851153146},
						{Labels: labels.FromStrings("dummyID", "5617"), Value: 25, HasTs: true, Ts: 1625851153146},
					},
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
					m: "test_histogram_family_count\xff__type__\xffhistogram\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"__type__", "histogram",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_sum\xff__type__\xffhistogram\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"__type__", "histogram",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "bar",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Length: 3},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, -1, 1},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"__type__", "histogram",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family_count\xff__type__\xffhistogram\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"__type__", "histogram",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_sum\xff__type__\xffhistogram\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"__type__", "histogram",
						"foo", "baz",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbaz\xffle\xff1.1",
					v: 0,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{
					m: "test_histogram_family_bucket\xff__type__\xffhistogram\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"__type__", "histogram",
						"foo", "baz",
						"le", "+Inf",
					),
				},
				{
					m: "test_histogram_family\xff__type__\xffhistogram\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           -53,
						PositiveSpans: []histogram.Span{
							{Offset: 1, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{5, -4},
						CustomValues:    []float64{1.1, 2.2},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"__type__", "histogram",
						"foo", "baz",
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
					m: "empty_histogram\xff__type__\xffhistogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
						"__type__", "histogram",
					),
				},
			},
		},
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("ignoreNative=%v,keepClassic=%v,typeAndUnit=%v", tc.ignoreNative, tc.keepClassic, tc.typeAndUnit)
		t.Run(name, func(t *testing.T) {
			p := NewProtobufParser(data, tc.ignoreNative, tc.keepClassic, true, tc.typeAndUnit, labels.NewSymbolTable())
			got := testParse(t, p)
			requireEntries(t, tc.expected, got)
		})
	}
}

func FuzzProtobufParser_Labels(f *testing.F) {
	// Add to the "seed corpus" the values that are known to reproduce issues
	// which this test has found in the past. These cases run during regular
	// testing, as well as the first step of the fuzzing process.
	f.Add(true, true, int64(123))
	f.Add(true, false, int64(129))
	f.Add(false, true, int64(159))
	f.Add(false, true, int64(-127))
	f.Fuzz(func(
		t *testing.T,
		parseClassicHistogram bool,
		enableTypeAndUnitLabels bool,
		randSeed int64,
	) {
		var (
			r              = rand.New(rand.NewSource(randSeed))
			buffers        = pool.New(1+r.Intn(128), 128+r.Intn(1024), 2, func(sz int) interface{} { return make([]byte, 0, sz) })
			lastScrapeSize = 0
			observedLabels []labels.Labels
			st             = labels.NewSymbolTable()
		)

		for i := 0; i < 20; i++ { // run multiple iterations to encounter memory corruptions
			// Get buffer from pool like in scrape.go
			b := buffers.Get(lastScrapeSize).([]byte)
			buf := bytes.NewBuffer(b)

			// Generate some scraped data to parse
			mf := generateFuzzMetricFamily(r)
			protoBuf, err := proto.Marshal(mf)
			require.NoError(t, err)
			sizeBuf := make([]byte, binary.MaxVarintLen32)
			sizeBufSize := binary.PutUvarint(sizeBuf, uint64(len(protoBuf)))
			buf.Write(sizeBuf[:sizeBufSize])
			buf.Write(protoBuf)

			// Use protobuf parser to parse like in real usage
			b = buf.Bytes()
			p := NewProtobufParser(b, false, parseClassicHistogram, false, enableTypeAndUnitLabels, st)

			for {
				entry, err := p.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				switch entry {
				case EntryHelp:
					name, help := p.Help()
					require.Equal(t, mf.Name, string(name))
					require.Equal(t, mf.Help, string(help))
				case EntryType:
					name, _ := p.Type()
					require.Equal(t, mf.Name, string(name))
				case EntryUnit:
					name, unit := p.Unit()
					require.Equal(t, mf.Name, string(name))
					require.Equal(t, mf.Unit, string(unit))
				case EntrySeries, EntryHistogram:
					var lbs labels.Labels
					p.Labels(&lbs)
					observedLabels = append(observedLabels, lbs)
				}

				// Get labels from exemplars
				for {
					var e exemplar.Exemplar
					if !p.Exemplar(&e) {
						break
					}
					observedLabels = append(observedLabels, e.Labels)
				}
			}

			// Validate all labels seen so far remain valid. This can find memory corruption issues.
			for _, l := range observedLabels {
				require.True(t, l.IsValid(model.LegacyValidation), "encountered corrupted labels: %v", l)
			}

			lastScrapeSize = len(b)
			buffers.Put(b)
		}
	})
}

func generateFuzzMetricFamily(
	r *rand.Rand,
) *dto.MetricFamily {
	unit := generateValidLabelName(r)
	metricName := fmt.Sprintf("%s_%s", generateValidMetricName(r), unit)
	metricTypeProto := dto.MetricType(r.Intn(len(dto.MetricType_name)))
	metricFamily := &dto.MetricFamily{
		Name: metricName,
		Help: generateHelp(r),
		Type: metricTypeProto,
		Unit: unit,
	}
	metricsCount := r.Intn(20)
	for i := 0; i < metricsCount; i++ {
		metric := dto.Metric{
			Label: generateFuzzLabels(r),
		}
		switch metricTypeProto {
		case dto.MetricType_GAUGE:
			metric.Gauge = &dto.Gauge{Value: r.Float64()}
		case dto.MetricType_COUNTER:
			metric.Counter = &dto.Counter{Value: r.Float64()}
		case dto.MetricType_SUMMARY:
			metric.Summary = &dto.Summary{Quantile: []dto.Quantile{{Quantile: 0.5, Value: r.Float64()}}}
		case dto.MetricType_HISTOGRAM:
			metric.Histogram = &dto.Histogram{Exemplars: generateExemplars(r)}
		}
		metricFamily.Metric = append(metricFamily.Metric, metric)
	}
	return metricFamily
}

func generateExemplars(r *rand.Rand) []*dto.Exemplar {
	exemplarsCount := r.Intn(5)
	exemplars := make([]*dto.Exemplar, 0, exemplarsCount)
	for i := 0; i < exemplarsCount; i++ {
		exemplars = append(exemplars, &dto.Exemplar{
			Label: generateFuzzLabels(r),
			Value: r.Float64(),
			Timestamp: &types.Timestamp{
				Seconds: int64(r.Intn(1000000000)),
				Nanos:   int32(r.Intn(1000000000)),
			},
		})
	}
	return exemplars
}

func generateFuzzLabels(r *rand.Rand) []dto.LabelPair {
	labelsCount := r.Intn(10)
	ls := make([]dto.LabelPair, 0, labelsCount)
	for i := 0; i < labelsCount; i++ {
		ls = append(ls, dto.LabelPair{
			Name:  generateValidLabelName(r),
			Value: generateValidLabelName(r),
		})
	}
	return ls
}

func generateHelp(r *rand.Rand) string {
	result := make([]string, 1+r.Intn(20))
	for i := 0; i < len(result); i++ {
		result[i] = generateValidLabelName(r)
	}
	return strings.Join(result, "_")
}

func generateValidLabelName(r *rand.Rand) string {
	return generateString(r, validFirstRunes, validLabelNameRunes)
}

func generateValidMetricName(r *rand.Rand) string {
	return generateString(r, validFirstRunes, validMetricNameRunes)
}

func generateString(r *rand.Rand, firstRunes, restRunes []rune) string {
	result := make([]rune, 1+r.Intn(20))
	for i := range result {
		if i == 0 {
			result[i] = firstRunes[r.Intn(len(firstRunes))]
		} else {
			result[i] = restRunes[r.Intn(len(restRunes))]
		}
	}
	return string(result)
}

var (
	validMetricNameRunes = []rune{
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'_', ':',
	}
	validLabelNameRunes = validMetricNameRunes[:len(validMetricNameRunes)-1] // skip the colon
	validFirstRunes     = validMetricNameRunes[:52]                          // only the letters
)
