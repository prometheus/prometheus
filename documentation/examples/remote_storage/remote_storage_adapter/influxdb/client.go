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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/prompb"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	logger log.Logger

	client          influx.Client
	database        string
	retentionPolicy string
	ignoredSamples  prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(logger log.Logger, conf influx.HTTPConfig, db string, rp string) *Client {
	c, err := influx.NewHTTPClient(conf)
	// Currently influx.NewClient() *should* never return an error.
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Client{
		logger:          logger,
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

// Write sends a batch of samples to InfluxDB via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	points := make([]*influx.Point, 0, len(samples))
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			level.Debug(c.logger).Log("msg", "cannot send  to InfluxDB, skipping sample", "value", v, "sample", s)
			c.ignoredSamples.Inc()
			continue
		}
		p, err := influx.NewPoint(
			string(s.Metric[model.MetricNameLabel]),
			tagsFromMetric(s.Metric),
			map[string]interface{}{"value": v},
			s.Timestamp.Time(),
		)
		if err != nil {
			return err
		}
		points = append(points, p)
	}

	bps, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Precision:       "ms",
		Database:        c.database,
		RetentionPolicy: c.retentionPolicy,
	})
	if err != nil {
		return err
	}
	bps.AddPoints(points)
	return c.client.Write(bps)
}

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	labelsToSeries := map[string]*prompb.TimeSeries{}
	for _, q := range req.Queries {
		command, err := c.buildCommand(q)
		if err != nil {
			return nil, err
		}

		query := influx.NewQuery(command, c.database, "ms")
		resp, err := c.client.Query(query)
		if err != nil {
			return nil, err
		}
		if resp.Err != "" {
			return nil, errors.New(resp.Err)
		}

		if err = mergeResult(labelsToSeries, resp.Results); err != nil {
			return nil, err
		}
	}

	resp := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{Timeseries: make([]*prompb.TimeSeries, 0, len(labelsToSeries))},
		},
	}
	for _, ts := range labelsToSeries {
		resp.Results[0].Timeseries = append(resp.Results[0].Timeseries, ts)
	}
	return &resp, nil
}

func (c *Client) buildCommand(q *prompb.Query) (string, error) {
	matchers := make([]string, 0, len(q.Matchers))
	// If we don't find a metric name matcher, query all metrics
	// (InfluxDB measurements) by default.
	from := "FROM /.+/"
	for _, m := range q.Matchers {
		if m.Name == model.MetricNameLabel {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				from = fmt.Sprintf("FROM %q.%q", c.retentionPolicy, m.Value)
			case prompb.LabelMatcher_RE:
				from = fmt.Sprintf("FROM %q./^%s$/", c.retentionPolicy, escapeSlashes(m.Value))
			default:
				// TODO: Figure out how to support these efficiently.
				return "", errors.New("non-equal or regex-non-equal matchers are not supported on the metric name yet")
			}
			continue
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ:
			matchers = append(matchers, fmt.Sprintf("%q = '%s'", m.Name, escapeSingleQuotes(m.Value)))
		case prompb.LabelMatcher_NEQ:
			matchers = append(matchers, fmt.Sprintf("%q != '%s'", m.Name, escapeSingleQuotes(m.Value)))
		case prompb.LabelMatcher_RE:
			matchers = append(matchers, fmt.Sprintf("%q =~ /^%s$/", m.Name, escapeSlashes(m.Value)))
		case prompb.LabelMatcher_NRE:
			matchers = append(matchers, fmt.Sprintf("%q !~ /^%s$/", m.Name, escapeSlashes(m.Value)))
		default:
			return "", errors.Errorf("unknown match type %v", m.Type)
		}
	}
	matchers = append(matchers, fmt.Sprintf("time >= %vms", q.StartTimestampMs))
	matchers = append(matchers, fmt.Sprintf("time <= %vms", q.EndTimestampMs))

	return fmt.Sprintf("SELECT value %s WHERE %v GROUP BY *", from, strings.Join(matchers, " AND ")), nil
}

func escapeSingleQuotes(str string) string {
	return strings.Replace(str, `'`, `\'`, -1)
}

func escapeSlashes(str string) string {
	return strings.Replace(str, `/`, `\/`, -1)
}

func mergeResult(labelsToSeries map[string]*prompb.TimeSeries, results []influx.Result) error {
	for _, r := range results {
		for _, s := range r.Series {
			k := concatLabels(s.Tags)
			ts, ok := labelsToSeries[k]
			if !ok {
				ts = &prompb.TimeSeries{
					Labels: tagsToLabelPairs(s.Name, s.Tags),
				}
				labelsToSeries[k] = ts
			}

			samples, err := valuesToSamples(s.Values)
			if err != nil {
				return err
			}

			ts.Samples = mergeSamples(ts.Samples, samples)
		}
	}
	return nil
}

func concatLabels(labels map[string]string) string {
	// 0xff cannot occur in valid UTF-8 sequences, so use it
	// as a separator here.
	separator := "\xff"
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+separator+v)
	}
	return strings.Join(pairs, separator)
}

func tagsToLabelPairs(name string, tags map[string]string) []prompb.Label {
	pairs := make([]prompb.Label, 0, len(tags))
	for k, v := range tags {
		if v == "" {
			// If we select metrics with different sets of labels names,
			// InfluxDB returns *all* possible tag names on all returned
			// series, with empty tag values on series where they don't
			// apply. In Prometheus, an empty label value is equivalent
			// to a non-existent label, so we just skip empty ones here
			// to make the result correct.
			continue
		}
		pairs = append(pairs, prompb.Label{
			Name:  k,
			Value: v,
		})
	}
	pairs = append(pairs, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: name,
	})
	return pairs
}

func valuesToSamples(values [][]interface{}) ([]prompb.Sample, error) {
	samples := make([]prompb.Sample, 0, len(values))
	for _, v := range values {
		if len(v) != 2 {
			return nil, errors.Errorf("bad sample tuple length, expected [<timestamp>, <value>], got %v", v)
		}

		jsonTimestamp, ok := v[0].(json.Number)
		if !ok {
			return nil, errors.Errorf("bad timestamp: %v", v[0])
		}

		jsonValue, ok := v[1].(json.Number)
		if !ok {
			return nil, errors.Errorf("bad sample value: %v", v[1])
		}

		timestamp, err := jsonTimestamp.Int64()
		if err != nil {
			return nil, errors.Wrap(err, "unable to convert sample timestamp to int64")
		}

		value, err := jsonValue.Float64()
		if err != nil {
			return nil, errors.Wrap(err, "unable to convert sample value to float64")
		}

		samples = append(samples, prompb.Sample{
			Timestamp: timestamp,
			Value:     value,
		})
	}
	return samples, nil
}

// mergeSamples merges two lists of sample pairs and removes duplicate
// timestamps. It assumes that both lists are sorted by timestamp.
func mergeSamples(a, b []prompb.Sample) []prompb.Sample {
	result := make([]prompb.Sample, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Timestamp < b[j].Timestamp {
			result = append(result, a[i])
			i++
		} else if a[i].Timestamp > b[j].Timestamp {
			result = append(result, b[j])
			j++
		} else {
			result = append(result, a[i])
			i++
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
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
