// Copyright 2015 The Prometheus Authors
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

package graphite

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

var (
	metric = model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"many_chars":          "abc!ABC:012-3!45ö67~89./(){},=.\"\\",
	}
)

func TestEscape(t *testing.T) {
	// Can we correctly keep and escape valid chars.
	value := "abzABZ019(){},'\"\\"
	expected := "abzABZ019\\(\\)\\{\\}\\,\\'\\\"\\\\"
	actual := escape(model.LabelValue(value))
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}

	// Test percent-encoding.
	value = "é/|_;:%."
	expected = "%C3%A9%2F|_;:%25%2E"
	actual = escape(model.LabelValue(value))
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestDecodeTargetJSON(t *testing.T) {
	exampleResponse := []byte(`[{
		"datapoints": [
		[
			null,
			null
		],
		[
			3.995926854361851,
			1510676572
		]
		],
		"tags": {
			"some": "tag",
			"name": "some_id_of_a_metric"
		}
	}]`)
	var data []TargetResponse
	err := json.Unmarshal(exampleResponse, &data)
	if err != nil {
		t.Errorf("Expected no error, got %s", err.Error())
	}
}

func TestPathFromMetric(t *testing.T) {
	expected := ("prefix." +
		"test:metric" +
		".many_chars.abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\" +
		".testlabel.test:value")
	actual := pathFromMetric(metric, "prefix.", false)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestPathFromMetricTagsEnabld(t *testing.T) {
	expected := ("prefix." +
		"test:metric" +
		";many_chars=abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\" +
		";testlabel=test:value")
	actual := pathFromMetric(metric, "prefix.", true)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestBuildPromSeries(t *testing.T) {
	tests := []struct {
		name         string
		targetSeries TargetResponse
		want         *prompb.TimeSeries
		wantErr      bool
	}{
		{
			name: "simple metric with null data point",
			targetSeries: TargetResponse{
				Tags: map[string]string{
					"some": "tag",
					"name": "some_metric",
				},
				DataPoints: [][2]json.Number{
					[2]json.Number{"100", "3.14"},
					[2]json.Number{"", ""},
				},
			},
			want: &prompb.TimeSeries{
				Samples: []*prompb.Sample{
					&prompb.Sample{
						Value:     100,
						Timestamp: 3140,
					},
				},
				Labels: []*prompb.Label{
					&prompb.Label{
						Value: "tag",
						Name:  "some",
					},
					&prompb.Label{
						Value: "some_metric",
						Name:  model.MetricNameLabel,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid metric",
			targetSeries: TargetResponse{
				Tags: map[string]string{
					"name": "random.carbon.metric",
				},
				DataPoints: [][2]json.Number{
					[2]json.Number{"100", "3.14"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPromSeries(tt.targetSeries)
			if err != nil && !tt.wantErr {
				t.Errorf("Error %v, want no error", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildPromSeries() = %v, want %v", got, tt.want)
			}
		})
	}
}
