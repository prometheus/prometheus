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
	"math"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	influx "github.com/influxdb/influxdb/client"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	client          *influx.Client
	database        string
	retentionPolicy string
	ignoredSamples  prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(conf influx.Config, db string, rp string) *Client {
	c, err := influx.NewClient(conf)
	// Currently influx.NewClient() *should* never return an error.
	if err != nil {
		log.Fatal(err)
	}

	return &Client{
		client:          c,
		database:        db,
		retentionPolicy: rp,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_influxdb_ignored_samples_total",
				Help: "The total number of samples not sent to InfluxDB due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}
}

// tagsFromMetric extracts InfluxDB tags from a Prometheus metric.
func tagsFromMetric(m model.Metric) map[string]string {
	tags := make(map[string]string, len(m)-1)
	for l, v := range m {
		if l != model.MetricNameLabel {
			tags[string(l)] = string(v)
		}
	}
	return tags
}

// Store sends a batch of samples to InfluxDB via its HTTP API.
func (c *Client) Store(samples model.Samples) error {
	points := make([]influx.Point, 0, len(samples))
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Debugf("cannot send value %f to InfluxDB, skipping sample %#v", v, s)
			c.ignoredSamples.Inc()
			continue
		}
		points = append(points, influx.Point{
			Measurement: string(s.Metric[model.MetricNameLabel]),
			Tags:        tagsFromMetric(s.Metric),
			Time:        s.Timestamp.Time(),
			Precision:   "ms",
			Fields: map[string]interface{}{
				"value": v,
			},
		})
	}

	bps := influx.BatchPoints{
		Points:          points,
		Database:        c.database,
		RetentionPolicy: c.retentionPolicy,
	}
	_, err := c.client.Write(bps)
	return err
}

// Name identifies the client as an InfluxDB client.
func (c Client) Name() string {
	return "influxdb"
}

// Describe implements prometheus.Collector.
func (c *Client) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ignoredSamples.Desc()
}

// Collect implements prometheus.Collector.
func (c *Client) Collect(ch chan<- prometheus.Metric) {
	ch <- c.ignoredSamples
}
