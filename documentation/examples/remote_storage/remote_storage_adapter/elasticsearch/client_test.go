// Copyright 2017 The Prometheus Authors
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

package elasticsearch

import (
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	elastic "gopkg.in/olivere/elastic.v3"
)

func mockElastic(url string) (*elastic.Client, error) {
	client, err := elastic.NewSimpleClient(elastic.SetURL(url))
	if err != nil {
		return nil, err
	}
	return client, nil
}

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

	expectedBody := `{"index":{"_index":"test-case","_type":"prom-metrics"}}
{"__name__":"testmetric","test_label":"test_label_value1","timestamp":"1973-11-29T21:33:09.123Z","value":1.23}
{"index":{"_index":"test-case","_type":"prom-metrics"}}
{"__name__":"testmetric","test_label":"test_label_value2","timestamp":"1973-11-29T21:33:09.123Z","value":5.1234}
`
	fakedBulkRequestResponse := `{
       "took": 30,
       "errors": false,
       "items": [
          {
             "index": {
                "_index": "test-case",
                "_type": "prom-metrics",
                "_id": "AV0XF6yTcgrDQLaUKggH",
                "_version": 1,
                "result": "created",
                "_shards": {
                   "total": 1,
                   "successful": 1,
                   "failed": 0
                },
                "created": true,
                "status": 201
             }
          },
          {
             "index": {
                "_index": "test-case",
                "_type": "prom-metrics",
                "_id": "AV0XF6yTcgrDQLaUKggI",
                "_version": 1,
                "result": "created",
                "_shards": {
                   "total": 1,
                   "successful": 1,
                   "failed": 0
                },
                "created": true,
                "status": 201
             }
          }
       ]
    }`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Fatalf("Unexpected method; expected POST, got %s", r.Method)
			}

			if r.URL.Path != "/_bulk" {
				t.Fatalf("Unexpected path; expected %s, got %s", "/_bulk", r.URL.Path)
			}
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Error reading body: %s", err)
			}

			if string(b) != expectedBody {
				t.Fatalf("Unexpected request body; expected:\n\n%s\n\ngot:\n\n%s", expectedBody, string(b))
			}

			// return the faked response
			w.Header().Set("Content-Type", "application/json;charset=utf-8")
			if _, err := io.WriteString(w, fakedBulkRequestResponse); err != nil {
				t.Fatalf("Failed to write response, err: %v", err)
			}

		},
	))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Unable to parse server URL %s: %s", server.URL, err)
	}

	es, err := mockElastic(serverURL.String())
	if err != nil {
		t.Fatalf("error creating elastic client to %s: %v", serverURL.String(), err)
	}

	c := &Client{
		client:  es,
		esIndex: "test-case",
		esType:  "prom-metrics",
		timeout: time.Minute,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_elasticsearch_ignored_samples_total",
				Help: "The total number of samples not sent to Elasticsearch due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}

	if err := c.Write(samples); err != nil {
		t.Fatalf("Error sending samples: %s", err)
	}
}
