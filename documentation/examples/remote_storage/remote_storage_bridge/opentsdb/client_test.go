// Copyright 2013 The Prometheus Authors
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

package opentsdb

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/prometheus/common/model"
)

var (
	metric = model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"many_chars":          "abc!ABC:012-3!45ö67~89./",
	}
)

func TestTagsFromMetric(t *testing.T) {
	expected := map[string]TagValue{
		"testlabel":  TagValue("test:value"),
		"many_chars": TagValue("abc!ABC:012-3!45ö67~89./"),
	}
	actual := tagsFromMetric(metric)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Expected %#v, got %#v", expected, actual)
	}
}

func TestMarshalStoreSamplesRequest(t *testing.T) {
	request := StoreSamplesRequest{
		Metric:    TagValue("test:metric"),
		Timestamp: 4711,
		Value:     3.1415,
		Tags:      tagsFromMetric(metric),
	}
	expectedJSON := []byte(`{"metric":"test_.metric","timestamp":4711,"value":3.1415,"tags":{"many_chars":"abc_21ABC_.012-3_2145_C3_B667_7E89./","testlabel":"test_.value"}}`)

	resultingJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal(request) resulted in err: %s", err)
	}
	if !bytes.Equal(resultingJSON, expectedJSON) {
		t.Errorf(
			"Marshal(request) => %q, want %q",
			resultingJSON, expectedJSON,
		)
	}

	var unmarshaledRequest StoreSamplesRequest
	err = json.Unmarshal(expectedJSON, &unmarshaledRequest)
	if err != nil {
		t.Fatalf("Unmarshal(expectedJSON, &unmarshaledRequest) resulted in err: %s", err)
	}
	if !reflect.DeepEqual(unmarshaledRequest, request) {
		t.Errorf(
			"Unmarshal(expectedJSON, &unmarshaledRequest) => %#v, want %#v",
			unmarshaledRequest, request,
		)
	}
}
