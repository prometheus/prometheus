// Copyright 2016 The Prometheus Authors
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

package generic

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestMarshalStoreSamplesRequest(t *testing.T) {

	expectedBody := "\n?\n\ntestmetric\x12\x1f\n\ntest_label\x12\x11test_label_value1\x1a\x10\t\xaeG\xe1z\x14\xae\xf3?\x10\x83\xb5\xe4\xf4\xcb\x03"

	ch := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Error reading body: %s", err)
		}

		if string(b) != expectedBody {
			t.Fatalf("Unexpected request body; expected: %q got: %q", expectedBody, string(b))
		}
		ch <- struct{}{}
	}))
	defer server.Close()

	samples := model.Samples{
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value1",
			},
			Timestamp: model.Time(123456789123),
			Value:     1.23,
		},
	}
	client := NewClient(server.URL, time.Second)
	if err := client.Store(samples); err != nil {
		t.Fatalf("Error sending samples: %s", err)
	}
	<-ch
}
