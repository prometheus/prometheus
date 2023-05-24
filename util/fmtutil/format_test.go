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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/prompb"
)

var writeRequestFixture = &prompb.WriteRequest{
	Metadata: []prompb.MetricMetadata{
		{
			MetricFamilyName: "test_metric1",
			Type:             2,
			Help:             "this is a test metric",
		},
	},
	Timeseries: []prompb.TimeSeries{
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

func TestParseMetricsTextAndFormat(t *testing.T) {
	input := bytes.NewReader([]byte(`
	# HELP test_metric1 this is a test metric
	# TYPE test_metric1 gauge
	test_metric1{b="c",baz="qux",d="e",foo="bar"} 1 1
	test_metric1{b="c",baz="qux",d="e",foo="bar"} 2 1
	`))
	labels := map[string]string{"job": "promtool"}

	expected, err := ParseMetricsTextAndFormat(input, labels)
	require.NoError(t, err)

	require.Equal(t, writeRequestFixture, expected)
}
