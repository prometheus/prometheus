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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	influx "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/query"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/prompb"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	logger *slog.Logger

	client         influx.Client
	organization   string
	bucket         string
	ignoredSamples prometheus.Counter

	context context.Context
}

// NewClient creates a new Client.
func NewClient(logger *slog.Logger, url, authToken, organization, bucket string) *Client {
	c := influx.NewClientWithOptions(
		url,
		authToken,
		influx.DefaultOptions().SetPrecision(time.Millisecond),
	)

	if logger == nil {
		logger = promslog.NewNopLogger()
	}

	return &Client{
		logger:       logger,
		client:       c,
		organization: organization,
		bucket:       bucket,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_influxdb_ignored_samples_total",
				Help: "The total number of samples not sent to InfluxDB due to unsupported float values (Inf, -Inf, NaN).",
			},
		),

		context: context.Background(),
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
	points := make([]*write.Point, 0, len(samples))
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			c.logger.Debug("Cannot send to InfluxDB, skipping sample", "value", v, "sample", s)
			c.ignoredSamples.Inc()
			continue
		}
		p := influx.NewPoint(
			string(s.Metric[model.MetricNameLabel]),
			tagsFromMetric(s.Metric),
			map[string]any{"value": v},
			s.Timestamp.Time(),
		)
		points = append(points, p)
	}

	writeAPI := c.client.WriteAPIBlocking(c.organization, c.bucket)
	writeAPI.EnableBatching() // default 5_000
	var err error
	for _, p := range points {
		if err = writeAPI.WritePoint(c.context, p); err != nil {
			return err
		}
	}
	if err = writeAPI.Flush(c.context); err != nil {
		return err
	}

	return nil
}

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	queryAPI := c.client.QueryAPI(c.organization)

	labelsToSeries := map[string]*prompb.TimeSeries{}
	for _, q := range req.Queries {
		command, err := c.buildCommand(q)
		if err != nil {
			return nil, err
		}

		resp, err := queryAPI.Query(c.context, command)
		if err != nil {
			return nil, err
		}
		if resp.Err() != nil {
			return nil, resp.Err()
		}

		for resp.Next() {
			if err = mergeResult(labelsToSeries, resp.Record()); err != nil {
				return nil, err
			}
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
	rangeInNs := fmt.Sprintf("start: time(v: %v), stop: time(v: %v)", q.StartTimestampMs*time.Millisecond.Nanoseconds(), q.EndTimestampMs*time.Millisecond.Nanoseconds())

	// If we don't find a metric name matcher, query all metrics
	// (InfluxDB measurements) by default.
	var measurement strings.Builder
	measurement.WriteString(`r._measurement`)
	matchers := make([]string, 0, len(q.Matchers))
	var joinedMatchers string
	for _, m := range q.Matchers {
		if m.Name == model.MetricNameLabel {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				measurement.WriteString(fmt.Sprintf(" == \"%s\"", m.Value))
			case prompb.LabelMatcher_RE:
				measurement.WriteString(fmt.Sprintf(" =~ /%s/", escapeSlashes(m.Value)))
			default:
				// TODO: Figure out how to support these efficiently.
				return "", errors.New("non-equal or regex-non-equal matchers are not supported on the metric name yet")
			}
			continue
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ:
			matchers = append(matchers, fmt.Sprintf("r.%s == \"%s\"", m.Name, escapeSingleQuotes(m.Value)))
		case prompb.LabelMatcher_NEQ:
			matchers = append(matchers, fmt.Sprintf("r.%s != \"%s\"", m.Name, escapeSingleQuotes(m.Value)))
		case prompb.LabelMatcher_RE:
			matchers = append(matchers, fmt.Sprintf("r.%s =~ /%s/", m.Name, escapeSingleQuotes(m.Value)))
		case prompb.LabelMatcher_NRE:
			matchers = append(matchers, fmt.Sprintf("r.%s !~ /%s/", m.Name, escapeSingleQuotes(m.Value)))
		default:
			return "", fmt.Errorf("unknown match type %v", m.Type)
		}
	}
	if len(matchers) > 0 {
		joinedMatchers = fmt.Sprintf(" and %s", strings.Join(matchers, " and "))
	}

	// _measurement must be retained, otherwise "invalid metric name" shall be thrown
	command := fmt.Sprintf(
		"from(bucket: \"%s\") |> range(%s) |> filter(fn: (r) => %s%s)",
		c.bucket, rangeInNs, measurement.String(), joinedMatchers,
	)

	return command, nil
}

func escapeSingleQuotes(str string) string {
	return strings.ReplaceAll(str, `'`, `\'`)
}

func escapeSlashes(str string) string {
	return strings.ReplaceAll(str, `/`, `\/`)
}

func mergeResult(labelsToSeries map[string]*prompb.TimeSeries, record *query.FluxRecord) error {
	builtIntime := record.Time()
	builtInvalue := record.Value()
	builtInMeasurement := record.Measurement()
	labels := record.Values()

	filterOutBuiltInLabels(labels)

	k := concatLabels(labels)

	ts, ok := labelsToSeries[k]
	if !ok {
		ts = &prompb.TimeSeries{
			Labels: tagsToLabelPairs(builtInMeasurement, labels),
		}
		labelsToSeries[k] = ts
	}

	sample, err := valuesToSamples(builtIntime, builtInvalue)
	if err != nil {
		return err
	}

	ts.Samples = mergeSamples(ts.Samples, []prompb.Sample{sample})

	return nil
}

func filterOutBuiltInLabels(labels map[string]any) {
	delete(labels, "table")
	delete(labels, "_start")
	delete(labels, "_stop")
	delete(labels, "_time")
	delete(labels, "_value")
	delete(labels, "_field")
	delete(labels, "result")
	delete(labels, "_measurement")
}

func concatLabels(labels map[string]any) string {
	// 0xff cannot occur in valid UTF-8 sequences, so use it
	// as a separator here.
	separator := "\xff"
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, fmt.Sprintf("%s%s%v", k, separator, v))
	}
	return strings.Join(pairs, separator)
}

func tagsToLabelPairs(name string, tags map[string]any) []prompb.Label {
	pairs := make([]prompb.Label, 0, len(tags))
	for k, v := range tags {
		if v == nil {
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
			Value: fmt.Sprintf("%v", v),
		})
	}
	pairs = append(pairs, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: name,
	})
	return pairs
}

func valuesToSamples(timestamp time.Time, value any) (prompb.Sample, error) {
	var valueFloat64 float64
	var valueInt64 int64
	var ok bool
	if valueFloat64, ok = value.(float64); !ok {
		valueInt64, ok = value.(int64)
		if !ok {
			return prompb.Sample{}, fmt.Errorf("unable to convert sample value to float64: %v", value)
		}
		valueFloat64 = float64(valueInt64)
	}

	return prompb.Sample{
		Timestamp: timestamp.UnixMilli(),
		Value:     valueFloat64,
	}, nil
}

// mergeSamples merges two lists of sample pairs and removes duplicate
// timestamps. It assumes that both lists are sorted by timestamp.
func mergeSamples(a, b []prompb.Sample) []prompb.Sample {
	result := make([]prompb.Sample, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i].Timestamp < b[j].Timestamp:
			result = append(result, a[i])
			i++
		case a[i].Timestamp > b[j].Timestamp:
			result = append(result, b[j])
			j++
		default:
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
func (Client) Name() string {
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
