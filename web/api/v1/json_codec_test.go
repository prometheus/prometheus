// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestJsonCodec_Encode(t *testing.T) {
	cases := []struct {
		response interface{}
		expected string
	}{
		{
			response: &QueryData{
				ResultType: parser.ValueTypeVector,
				Result: promql.Vector{
					promql.Sample{
						Metric: labels.FromStrings("__name__", "foo"),
						T:      1000,
						F:      1,
					},
					promql.Sample{
						Metric: labels.FromStrings("__name__", "bar"),
						T:      2000,
						F:      2,
					},
				},
			},
			expected: `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"foo"},"value":[1,"1"]},{"metric":{"__name__":"bar"},"value":[2,"2"]}]}}`,
		},
		{
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Metric: labels.FromStrings("__name__", "foo"),
						Floats: []promql.FPoint{{F: 1, T: 1000}},
					},
					promql.Series{
						Metric: labels.FromStrings("__name__", "bar"),
						Floats: []promql.FPoint{{F: 2, T: 2000}},
					},
				},
			},
			expected: `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"foo"},"values":[[1,"1"]]},{"metric":{"__name__":"bar"},"values":[[2,"2"]]}]}}`,
		},
		{
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Floats: []promql.FPoint{{F: 1, T: 1000}},
						Metric: labels.FromStrings("__name__", "foo"),
					},
				},
			},
			expected: `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"foo"},"values":[[1,"1"]]}]}}`,
		},
		{
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Histograms: []promql.HPoint{{H: &histogram.FloatHistogram{
							Schema:        2,
							ZeroThreshold: 0.001,
							ZeroCount:     12,
							Count:         10,
							Sum:           20,
							PositiveSpans: []histogram.Span{
								{Offset: 3, Length: 2},
								{Offset: 1, Length: 3},
							},
							NegativeSpans: []histogram.Span{
								{Offset: 2, Length: 2},
							},
							PositiveBuckets: []float64{1, 2, 2, 1, 1},
							NegativeBuckets: []float64{2, 1},
						}, T: 1000}},
						Metric: labels.FromStrings("__name__", "foo"),
					},
				},
			},
			expected: `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"foo"},"histograms":[[1,{"count":"10","sum":"20","buckets":[[1,"-1.6817928305074288","-1.414213562373095","1"],[1,"-1.414213562373095","-1.189207115002721","2"],[3,"-0.001","0.001","12"],[0,"1.414213562373095","1.6817928305074288","1"],[0,"1.6817928305074288","2","2"],[0,"2.378414230005442","2.82842712474619","2"],[0,"2.82842712474619","3.3635856610148576","1"],[0,"3.3635856610148576","4","1"]]}]]}]}}`,
		},
		{
			response: promql.FPoint{F: 0, T: 0},
			expected: `{"status":"success","data":[0,"0"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 1},
			expected: `{"status":"success","data":[0.001,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 10},
			expected: `{"status":"success","data":[0.010,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 100},
			expected: `{"status":"success","data":[0.100,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 1001},
			expected: `{"status":"success","data":[1.001,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 1010},
			expected: `{"status":"success","data":[1.010,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 1100},
			expected: `{"status":"success","data":[1.100,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: 12345678123456555},
			expected: `{"status":"success","data":[12345678123456.555,"20"]}`,
		},
		{
			response: promql.FPoint{F: 20, T: -1},
			expected: `{"status":"success","data":[-0.001,"20"]}`,
		},
		{
			response: promql.FPoint{F: math.NaN(), T: 0},
			expected: `{"status":"success","data":[0,"NaN"]}`,
		},
		{
			response: promql.FPoint{F: math.Inf(1), T: 0},
			expected: `{"status":"success","data":[0,"+Inf"]}`,
		},
		{
			response: promql.FPoint{F: math.Inf(-1), T: 0},
			expected: `{"status":"success","data":[0,"-Inf"]}`,
		},
		{
			response: promql.FPoint{F: 1.2345678e6, T: 0},
			expected: `{"status":"success","data":[0,"1234567.8"]}`,
		},
		{
			response: promql.FPoint{F: 1.2345678e-6, T: 0},
			expected: `{"status":"success","data":[0,"0.0000012345678"]}`,
		},
		{
			response: promql.FPoint{F: 1.2345678e-67, T: 0},
			expected: `{"status":"success","data":[0,"1.2345678e-67"]}`,
		},
		{
			response: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("foo", "bar"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("trace_id", "abc"),
							Value:  100.123,
							Ts:     1234,
						},
					},
				},
			},
			expected: `{"status":"success","data":[{"seriesLabels":{"foo":"bar"},"exemplars":[{"labels":{"trace_id":"abc"},"value":"100.123","timestamp":1.234}]}]}`,
		},
		{
			response: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("foo", "bar"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("trace_id", "abc"),
							Value:  math.Inf(1),
							Ts:     1234,
						},
					},
				},
			},
			expected: `{"status":"success","data":[{"seriesLabels":{"foo":"bar"},"exemplars":[{"labels":{"trace_id":"abc"},"value":"+Inf","timestamp":1.234}]}]}`,
		},
	}

	codec := JSONCodec{}

	for _, c := range cases {
		body, err := codec.Encode(&Response{
			Status: statusSuccess,
			Data:   c.response,
		})
		if err != nil {
			t.Fatalf("Error encoding response body: %s", err)
		}

		if string(body) != c.expected {
			t.Fatalf("Expected response \n%v\n but got \n%v\n", c.expected, string(body))
		}
	}
}
