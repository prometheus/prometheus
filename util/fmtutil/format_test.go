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
	"math"
	"testing"
	"time"

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
				{Name: "__name__", Value: "http_request_duration_seconds_sum"},
				{Name: "job", Value: "promtool"},
			},
			Samples: []prompb.Sample{{Value: 53423, Timestamp: 1}},
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

	require.Equal(t, expected, writeRequestFixture)
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
	require.Equal(t, "text format parsing error in line 4: expected float as value, got \"1027Error\"", err.Error())
}

func TestMetricTextToWriteRequestErrorParsingMetricType(t *testing.T) {
	input := bytes.NewReader([]byte(`
	# HELP node_info node info summary.
	# TYPE node_info info
	node_info{test="summary"} 1 1395066363000
	`))
	labels := map[string]string{"job": "promtool"}

	_, err := MetricTextToWriteRequest(input, labels)
	require.Equal(t, "text format parsing error in line 3: unknown metric type \"info\"", err.Error())
}

func TestMakeTimeseries_HistogramInfBucket(t *testing.T) {
	tests := map[string]*dto.Histogram{
		"Histogram missing +Inf bucket": {
			Bucket: []*dto.Bucket{
				{CumulativeCount: p[uint64](5), UpperBound: p(1.0)},
				{CumulativeCount: p[uint64](10), UpperBound: p(5.0)},
			},
			SampleCount: p[uint64](15),
		},
		"Histogram already has +Inf bucket": {
			Bucket: []*dto.Bucket{
				{CumulativeCount: p[uint64](5), UpperBound: p(1.0)},
				{CumulativeCount: p[uint64](10), UpperBound: p(5.0)},
				{CumulativeCount: p[uint64](15), UpperBound: p(math.Inf(1))},
			},
			SampleCount: p[uint64](15),
		},
	}

	for name, histogram := range tests {
		t.Run(name, func(t *testing.T) {
			wr := &prompb.WriteRequest{}
			labels := map[string]string{"__name__": "test_histogram"}
			metric := &dto.Metric{
				Histogram:   histogram,
				TimestampMs: p(time.Now().UnixMilli()),
			}

			require.NoError(t, makeTimeseries(wr, labels, metric))

			var hasInf bool
			for _, ts := range wr.Timeseries {
				for _, lbl := range ts.Labels {
					if lbl.Name == "le" && lbl.Value == "+Inf" {
						hasInf = true
					}
				}
			}
			require.Truef(t, hasInf, "expected +Inf bucket in histogram")
		})
	}
}

func p[T any](v T) *T {
	return &v
}
