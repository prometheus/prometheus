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
	"encoding/json"
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	elastic "gopkg.in/olivere/elastic.v3"
)

// Client allows sending batches of Prometheus samples to ElasticSearch.
type Client struct {
	client         *elastic.Client
	esIndex        string
	esType         string
	timeout        time.Duration
	ignoredSamples prometheus.Counter
}

// fieldsFromMetric extracts Elastic fields from a Prometheus metric.
// In elasticsearch, `__name__` could also be a simple field.
func fieldsFromMetric(m model.Metric) map[string]interface{} {
	fields := make(map[string]interface{}, len(m))
	for l, v := range m {
		fields[string(l)] = string(v)
	}
	return fields
}

func generateEsIndex(esIndexPerfix string) string {
	var separator = "-"
	dateSuffix := time.Now().Format("2006-01-02")
	return esIndexPerfix + separator + dateSuffix
}

// NewClient return a Client which contains an elasticsearch client.
// Now, it generates the real esIndex formatted with `<esIndexPerfix>-YYYY-mm-dd`.
func NewClient(url string, maxRetries int, esIndexPerfix, esType string, timeout time.Duration) *Client {
	client, err := elastic.NewClient(
		elastic.SetURL(url),
		elastic.SetMaxRetries(maxRetries),
		// TODO: add basic auth support.
	)
	if err != nil {
		log.Fatal(err)
	}

	// Use the IndexExists service to check if a specified index exists.
	esIndex := generateEsIndex(esIndexPerfix)
	exists, err := client.IndexExists(esIndex).Do()
	if err != nil {
		log.Errorf("index %v is not found in Elastic.", esIndex)
	}

	// Create an index if it is not exist.
	if !exists {
		// Create a new index.
		createIndex, err := client.CreateIndex(esIndex).Do()
		if err != nil {
			log.Fatalf("failed to create index %v, err: %v", esIndex, err)
		}
		if !createIndex.Acknowledged {
			// Not acknowledged
			log.Fatalf("elasticsearch no acknowledged.")
		}
	}

	return &Client{
		client:  client,
		esIndex: esIndex,
		esType:  esType,
		timeout: timeout,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_elasticsearch_ignored_samples_total",
				Help: "The total number of samples not sent to Elasticsearch due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}
}

// Write sends a batch of samples to Graphite.
func (c *Client) Write(samples model.Samples) error {
	bulkRequest := c.client.Bulk().Timeout(c.timeout.String())

	for _, s := range samples {
		document := make(map[string]interface{}, len(s.Metric)+2)

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Debugf("cannot send value %f to Elasticsearch, skipping sample %#v", v, s)
			c.ignoredSamples.Inc()
			continue
		}

		document = fieldsFromMetric(s.Metric)
		document["value"] = v
		document["timestamp"] = s.Timestamp.Time()
		documentJSON, err := json.Marshal(document)
		if err != nil {
			log.Debugf("error while marshaling document, err: %v", err)
			continue
		}

		indexRq := elastic.NewBulkIndexRequest().
			Index(c.esIndex).
			Type(c.esType).
			Doc(string(documentJSON))
		bulkRequest = bulkRequest.Add(indexRq)
	}

	bulkResponse, err := bulkRequest.Do()
	if err != nil {
		return err
	}

	// there are some failed requests, count it!
	failedResults := bulkResponse.Failed()
	log.Debugf("there are %d failed requests to Elasticsearch.", len(failedResults))
	for _ = range failedResults {
		c.ignoredSamples.Inc()
	}
	return nil
}

// Name identifies the client as an elasticsearch client.
func (c Client) Name() string {
	return "elasticsearch"
}

// Describe implements prometheus.Collector.
func (c *Client) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ignoredSamples.Desc()
}

// Collect implements prometheus.Collector.
func (c *Client) Collect(ch chan<- prometheus.Metric) {
	ch <- c.ignoredSamples
}
