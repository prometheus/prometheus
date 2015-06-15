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

package influxdb

import (
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

func TestClient(t *testing.T) {
	samples := clientmodel.Samples{
		{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testmetric",
				"test_label":                "test_label_value1",
			},
			Timestamp: clientmodel.Timestamp(123456789123),
			Value:     1.23,
		},
		{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testmetric",
				"test_label":                "test_label_value2",
			},
			Timestamp: clientmodel.Timestamp(123456789123),
			Value:     5.1234,
		},
		{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "special_float_value",
			},
			Timestamp: clientmodel.Timestamp(123456789123),
			Value:     clientmodel.SampleValue(math.NaN()),
		},
	}

	expectedJSON := `{"database":"prometheus","retentionPolicy":"default","points":[{"timestamp":123456789123000000,"precision":"n","name":"testmetric","tags":{"test_label":"test_label_value1"},"fields":{"value":"1.23"}},{"timestamp":123456789123000000,"precision":"n","name":"testmetric","tags":{"test_label":"test_label_value2"},"fields":{"value":"5.1234"}}]}`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Fatalf("Unexpected method; expected POST, got %s", r.Method)
			}
			if r.URL.Path != writeEndpoint {
				t.Fatalf("Unexpected path; expected %s, got %s", writeEndpoint, r.URL.Path)
			}
			ct := r.Header["Content-Type"]
			if len(ct) != 1 {
				t.Fatalf("Unexpected number of 'Content-Type' headers; got %d, want 1", len(ct))
			}
			if ct[0] != contentTypeJSON {
				t.Fatalf("Unexpected 'Content-type'; expected %s, got %s", contentTypeJSON, ct[0])
			}
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Error reading body: %s", err)
			}

			if string(b) != expectedJSON {
				t.Fatalf("Unexpected request body; expected:\n\n%s\n\ngot:\n\n%s", expectedJSON, string(b))
			}
		},
	))
	defer server.Close()

	c := NewClient(server.URL, time.Minute, "prometheus", "default")

	if err := c.Store(samples); err != nil {
		t.Fatalf("Error sending samples: %s", err)
	}
}
