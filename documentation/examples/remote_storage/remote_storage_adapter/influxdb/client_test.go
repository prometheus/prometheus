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
	"net/url"
	"testing"
	"time"

	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	samples := model.Samples{
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value1",
			},
			Timestamp: model.Time(123456789123),
			Value:     1.23,
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value2",
			},
			Timestamp: model.Time(123456789123),
			Value:     5.1234,
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "nan_value",
			},
			Timestamp: model.Time(123456789123),
			Value:     model.SampleValue(math.NaN()),
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "pos_inf_value",
			},
			Timestamp: model.Time(123456789123),
			Value:     model.SampleValue(math.Inf(1)),
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "neg_inf_value",
			},
			Timestamp: model.Time(123456789123),
			Value:     model.SampleValue(math.Inf(-1)),
		},
	}

	expectedBody := `testmetric,test_label=test_label_value1 value=1.23 123456789123
testmetric,test_label=test_label_value2 value=5.1234 123456789123
`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "POST", r.Method, "Unexpected method.")
			require.Equal(t, "/write", r.URL.Path, "Unexpected path.")
			b, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err, "Error reading body.")
			require.Equal(t, expectedBody, string(b), "Unexpected request body.")
		},
	))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err, "Unable to parse server URL.")

	conf := influx.HTTPConfig{
		Addr:     serverURL.String(),
		Username: "testuser",
		Password: "testpass",
		Timeout:  time.Minute,
	}
	c := NewClient(nil, conf, "test_db", "default")
	err = c.Write(samples)
	require.NoError(t, err, "Error sending samples.")
}
