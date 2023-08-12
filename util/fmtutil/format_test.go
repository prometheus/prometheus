// Copyright 2023 The Prometheus Authors
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

package fmtutil

import (
	"bytes"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/prompb"
)

var writeRequestFixture = &prompb.WriteRequest{
	Metadata: []prompb.MetricMetadata{
		{
			MetricFamilyName: "http_request_duration_seconds",
			Type:             3,
			Help:             "A histogram of the request duration.",
		},
		{
			MetricFamilyName: "http_requests_total",
			Type:             1,
			Help:             "The total number of HTTP requests.",
		},
		{
			MetricFamilyName: "rpc_duration_seconds",
			Type:             5,
			Help:             "A summary of the RPC duration in seconds.",
		},
		{
			MetricFamilyName: "test_metric1",
			Type:             2,
			Help:             "This is a test metric.",
		},
	},
	Timeseries: []prompb.TimeSeries{
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_bucket"},
				{Name: "job", Value: "promtool"},
				{Name: "le", Value: "0.1"},
			},
			Samples: []prompb.Sample{{Value: 33444, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_bucket"},
				{Name: "job", Value: "promtool"},
				{Name: "le", Value: "0.5"},
			},
			Samples: []prompb.Sample{{Value: 129389, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_bucket"},
				{Name: "job", Value: "promtool"},
				{Name: "le", Value: "1"},
			},
			Samples: []prompb.Sample{{Value: 133988, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_bucket"},
				{Name: "job", Value: "promtool"},
				{Name: "le", Value: "+Inf"},
			},
			Samples: []prompb.Sample{{Value: 144320, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_count"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 144320, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_request_duration_seconds_sum"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 53423, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_requests_total"},
				{Name: "code", Value: "200"},
				{Name: "job", Value: "promtool"},
				{Name: "method", Value: "post"},
			},
			Samples: []prompb.Sample{{Value: 1027, Timestamp: 1395066363000}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "http_requests_total"},
				{Name: "code", Value: "400"},
				{Name: "job", Value: "promtool"},
				{Name: "method", Value: "post"},
			},
			Samples: []prompb.Sample{{Value: 3, Timestamp: 1395066363000}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "rpc_duration_seconds"},
				{Name: "job", Value: "promtool"},
				{Name: "quantile", Value: "0.01"},
			},
			Samples: []prompb.Sample{{Value: 3102, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "rpc_duration_seconds"},
				{Name: "job", Value: "promtool"},
				{Name: "quantile", Value: "0.5"},
			},
			Samples: []prompb.Sample{{Value: 4773, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "rpc_duration_seconds"},
				{Name: "job", Value: "promtool"},
				{Name: "quantile", Value: "0.99"},
			},
			Samples: []prompb.Sample{{Value: 76656, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "rpc_duration_seconds_sum"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 1.7560473e+07, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "rpc_duration_seconds_count"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 2693, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 1, Timestamp: 1}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 2, Timestamp: 1}},
		},
	},
}

func TestParseAndPushMetricsTextAndFormat(t *testing.T) {
	input := bytes.NewReader([]byte(`
	# HELP http_request_duration_seconds A histogram of the request duration.
	# TYPE http_request_duration_seconds histogram
	http_request_duration_seconds_bucket{le="0.1"} 33444 1
	http_request_duration_seconds_bucket{le="0.5"} 129389 1
	http_request_duration_seconds_bucket{le="1"} 133988 1
	http_request_duration_seconds_bucket{le="+Inf"} 144320 1
	http_request_duration_seconds_sum 53423 1
	http_request_duration_seconds_count 144320 1
	# HELP http_requests_total The total number of HTTP requests.
	# TYPE http_requests_total counter
	http_requests_total{method="post",code="200"} 1027 1395066363000
	http_requests_total{method="post",code="400"}    3 1395066363000
	# HELP rpc_duration_seconds A summary of the RPC duration in seconds.
	# TYPE rpc_duration_seconds summary
	rpc_duration_seconds{quantile="0.01"} 3102 1
	rpc_duration_seconds{quantile="0.5"} 4773 1
	rpc_duration_seconds{quantile="0.99"} 76656 1
	rpc_duration_seconds_sum 1.7560473e+07 1
	rpc_duration_seconds_count 2693 1
	# HELP test_metric1 This is a test metric.
	# TYPE test_metric1 gauge
	test_metric1{b="c",baz="qux",d="e",foo="bar"} 1 1
	test_metric1{b="c",baz="qux",d="e",foo="bar"} 2 1
	`))
	labels := map[string]string{"job": "promtool"}

	expected, err := MetricTextToWriteRequest(input, labels)
	require.NoError(t, err)

	require.Equal(t, writeRequestFixture, expected)
}

func TestMetricTextToWriteRequestErrorParsingFloatValue(t *testing.T) {
	input := bytes.NewReader([]byte(`
	# HELP http_requests_total The total number of HTTP requests.
	# TYPE http_requests_total counter
	http_requests_total{method="post",code="200"} 1027Error 1395066363000
	http_requests_total{method="post",code="400"}    3 1395066363000
	`))
	labels := map[string]string{"job": "promtool"}

	_, err := MetricTextToWriteRequest(input, labels)
	require.Equal(t, err.Error(), "text format parsing error in line 4: expected float as value, got \"1027Error\"")
}

func TestMetricTextToWriteRequestErrorParsingMetricType(t *testing.T) {
	input := bytes.NewReader([]byte(`
	# HELP node_info node info summary.
	# TYPE node_info info
	node_info{test="summary"} 1 1395066363000
	`))
	labels := map[string]string{"job": "promtool"}

	_, err := MetricTextToWriteRequest(input, labels)
	require.Equal(t, err.Error(), "text format parsing error in line 3: unknown metric type \"info\"")
}

func TestMetricFamiliesToWriteRequestWithHistograms(t *testing.T) {
	const TS int64 = 42

	type input struct {
		generate func(*testing.T) []*dto.MetricFamily
	}

	type expected struct {
		metadata   []prompb.MetricMetadata
		timeseries []prompb.TimeSeries
	}

	testcases := map[string]struct {
		input    input
		expected expected
	}{
		"classic": {
			input: input{
				generate: func(t *testing.T) []*dto.MetricFamily {
					h := prometheus.NewHistogram(prometheus.HistogramOpts{
						Namespace:   "ns",
						Subsystem:   "ss",
						Name:        "classic",
						Help:        "ns_ss_classic help",
						ConstLabels: prometheus.Labels{"l1": "v1"},
						Buckets:     []float64{0.1, 1.0, 10.0},
					})

					r := prometheus.NewPedanticRegistry()
					r.MustRegister(h)

					h.Observe(0.05)
					h.Observe(0.5)
					h.Observe(5)
					h.Observe(50)

					mfs, err := r.Gather()
					require.NoError(t, err)

					return mfs
				},
			},
			expected: expected{
				metadata: []prompb.MetricMetadata{
					{
						Type:             prompb.MetricMetadata_HISTOGRAM,
						MetricFamilyName: "ns_ss_classic",
						Help:             "ns_ss_classic help",
					},
				},
				timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_classic_bucket"},
							{Name: "l1", Value: "v1"},
							{Name: "le", Value: "0.1"},
						},
						Samples: []prompb.Sample{
							{Timestamp: TS, Value: 1},
						},
					},
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_classic_bucket"},
							{Name: "l1", Value: "v1"},
							{Name: "le", Value: "1"},
						},
						Samples: []prompb.Sample{
							{Timestamp: TS, Value: 2},
						},
					},
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_classic_bucket"},
							{Name: "l1", Value: "v1"},
							{Name: "le", Value: "10"},
						},
						Samples: []prompb.Sample{
							{Timestamp: TS, Value: 3},
						},
					},
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_classic_count"},
							{Name: "l1", Value: "v1"},
						},
						Samples: []prompb.Sample{
							{Timestamp: TS, Value: 4},
						},
					},
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_classic_sum"},
							{Name: "l1", Value: "v1"},
						},
						Samples: []prompb.Sample{
							{Timestamp: TS, Value: 55.55},
						},
					},
				},
			},
		},
		"native with integer counts": {
			input: input{
				generate: func(t *testing.T) []*dto.MetricFamily {
					h := prometheus.NewHistogram(prometheus.HistogramOpts{
						Namespace:                    "ns",
						Subsystem:                    "ss",
						Name:                         "native",
						Help:                         "ns_ss_native help",
						ConstLabels:                  prometheus.Labels{"l1": "v1"},
						NativeHistogramBucketFactor:  1.1,
						NativeHistogramZeroThreshold: 0.1,
					})

					r := prometheus.NewPedanticRegistry()
					r.MustRegister(h)

					h.Observe(0.05)
					h.Observe(0.5)
					h.Observe(5)
					h.Observe(50)

					mfs, err := r.Gather()
					require.NoError(t, err)

					return mfs
				},
			},
			expected: expected{
				metadata: []prompb.MetricMetadata{
					{
						Type:             prompb.MetricMetadata_HISTOGRAM,
						MetricFamilyName: "ns_ss_native",
						Help:             "ns_ss_native help",
					},
				},
				timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "ns_ss_native"},
							{Name: "l1", Value: "v1"},
						},
						Samples: nil,
						Histograms: []prompb.Histogram{
							{
								// There's _a lot_ of magic here. The count, zero count and sum
								// can be trivially deduced from the generating function. The
								// ZeroThreshold is an input.
								//
								// The schema, negative spans, positive spans and positie deltas
								// are computed by the histogram.
								Count:         &prompb.Histogram_CountInt{CountInt: 4},
								Sum:           55.55,
								Schema:        3,
								ZeroThreshold: 0.1,
								ZeroCount:     &prompb.Histogram_ZeroCountInt{ZeroCountInt: 1},
								NegativeSpans: []prompb.BucketSpan{},
								PositiveSpans: []prompb.BucketSpan{
									{Offset: -8, Length: 1},
									{Offset: 26, Length: 1},
									{Offset: 26, Length: 1},
								},
								PositiveDeltas: []int64{1, 0, 0},
								Timestamp:      TS,
							},
						},
					},
				},
			},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			mfs := testcase.input.generate(t)
			req := make(map[string]*dto.MetricFamily)
			ts := TS
			for _, mf := range mfs {
				// patch the timestamp so that the comparison succeeds
				for _, m := range mf.GetMetric() {
					m.TimestampMs = &ts
				}
				req[mf.GetName()] = mf
			}

			wr, err := MetricFamiliesToWriteRequest(req, nil)
			require.NoError(t, err)
			require.NotNil(t, wr)

			require.Equal(t, testcase.expected.metadata, wr.GetMetadata())
			require.Equal(t, testcase.expected.timeseries, wr.GetTimeseries())
		})
	}
}
