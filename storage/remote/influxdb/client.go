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
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/util/httputil"
)

const (
	writeEndpoint      = "/write"
	contentTypeDefault = "application/x-www-form-urlencoded"
	contentTypeJSON    = "application/json"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	url             string
	httpClient      *http.Client
	retentionPolicy string
	database        string
}

// NewClient creates a new Client.
func NewClient(url string, timeout time.Duration, database, retentionPolicy string) *Client {
	return &Client{
		url:             url,
		httpClient:      httputil.NewDeadlineClient(timeout, nil),
		retentionPolicy: retentionPolicy,
		database:        database,
	}
}

// StoreSamplesRequest is used for building a line protocol request for storing samples
// in InfluxDB.
type StoreSamplesRequest struct {
	Database        string
	RetentionPolicy string
	Points          []point
}

// point represents a single InfluxDB measurement.
type point struct {
	Time        int64
	Measurement clientmodel.LabelValue
	Tags        clientmodel.LabelSet
	Value       float64
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

// Convert float to a string with at least one decimal place
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// Convert points to Influxdb line protocol, buffer
func pointsToLineProtocol(points []point) bytes.Buffer {
	var pairs []string
	for _, point := range points {
		var line_s []string
		line_s = append(line_s, string(point.Measurement))
		for key, value := range point.Tags {
			line_s = append(line_s, string(key)+"="+string(value))
		}
		var line = strings.Join(line_s, ",") + " value=" + formatFloat(point.Value) + " " + strconv.FormatInt(point.Time, 10)
		pairs = append(pairs, line)
	}
	var b bytes.Buffer
	b.WriteString(strings.Join(pairs, "\n"))
	return b
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
			Time:        s.Timestamp.UnixNano(),
			Measurement: metric,
			Tags:        tagsFromMetric(s.Metric),
			Value:       v,
		},
		)
	}

	// Construct the http request
	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	u.Path = writeEndpoint
	values := u.Query()
	values.Set("precision", "n")
	values.Set("db", c.database)
	values.Set("rp", c.retentionPolicy)
	u.RawQuery = values.Encode()

	linesBuffer := pointsToLineProtocol(points)
	req, err := http.NewRequest("POST", u.String(), &linesBuffer)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeDefault)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// API returns status code 204 for successful writes.
	// http://influxdb.com/docs/v0.9/concepts/reading_and_writing_data.html#response
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	// API returns error details in the response content as text.
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return fmt.Errorf("failed to write samples into InfluxDB. Error CODE=%v: %s", resp.StatusCode, buf)
}

// Name identifies the client as an InfluxDB client.
func (c Client) Name() string {
	return "influxdb"
}
