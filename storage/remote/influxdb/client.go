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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/util/httputil"
)

const (
	writeEndpoint   = "/write"
	contentTypeJSON = "application/json"
)

var (
	retentionPolicy = flag.String("storage.remote.influxdb.retention-policy", "default", "The InfluxDB retention policy to use.")
	database        = flag.String("storage.remote.influxdb.database", "prometheus", "The name of the database to use for storing samples in InfluxDB.")
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	url        string
	httpClient *http.Client
}

// NewClient creates a new Client.
func NewClient(url string, timeout time.Duration) *Client {
	return &Client{
		url:        url,
		httpClient: httputil.NewDeadlineClient(timeout),
	}
}

// StoreSamplesRequest is used for building a JSON request for storing samples
// in InfluxDB.
type StoreSamplesRequest struct {
	Database        string  `json:"database"`
	RetentionPolicy string  `json:"retentionPolicy"`
	Points          []point `json:"points"`
}

// point represents a single InfluxDB measurement.
type point struct {
	Timestamp int64                  `json:"timestamp"`
	Precision string                 `json:"precision"`
	Name      clientmodel.LabelValue `json:"name"`
	Tags      clientmodel.LabelSet   `json:"tags"`
	Fields    fields                 `json:"fields"`
}

// fields represents the fields/columns sent to InfluxDB for a given measurement.
type fields struct {
	Value clientmodel.SampleValue `json:"value"`
}

// tagsFromMetric extracts InfluxDB tags from a Prometheus metric.
func tagsFromMetric(m clientmodel.Metric) clientmodel.LabelSet {
	tags := make(clientmodel.LabelSet, len(m)-1)
	for l, v := range m {
		if l == clientmodel.MetricNameLabel {
			continue
		}
		tags[l] = v
	}
	return tags
}

// Store sends a batch of samples to InfluxDB via its HTTP API.
func (c *Client) Store(samples clientmodel.Samples) error {
	points := make([]point, 0, len(samples))
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			// TODO(julius): figure out if it's possible to insert special float
			// values into InfluxDB somehow.
			log.Warnf("cannot send value %f to InfluxDB, skipping sample %#v", v, s)
			continue
		}
		metric := s.Metric[clientmodel.MetricNameLabel]
		points = append(points, point{
			Timestamp: s.Timestamp.UnixNano(),
			Precision: "n",
			Name:      metric,
			Tags:      tagsFromMetric(s.Metric),
			Fields: fields{
				Value: s.Value,
			},
		})
	}

	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}

	u.Path = writeEndpoint

	req := StoreSamplesRequest{
		Database:        *database,
		RetentionPolicy: *retentionPolicy,
		Points:          points,
	}
	buf, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(
		u.String(),
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// API returns status code 200 for successful writes.
	// http://influxdb.com/docs/v0.9/concepts/reading_and_writing_data.html#response
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// API returns error details in the response content in JSON.
	buf, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var r map[string]string
	if err := json.Unmarshal(buf, &r); err != nil {
		return err
	}
	return fmt.Errorf("failed to write samples into InfluxDB. Error: %s", r["error"])
}

// Name identifies the client as an InfluxDB client.
func (c Client) Name() string {
	return "influxdb"
}
