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

package clickhouse

import (
	"database/sql"
	"math"
	"os"
	"strings"

	_ "github.com/ClickHouse/clickhouse-go"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/prompb"
)

// Client allows sending batches of Prometheus samples to Clickhouse.
type Client struct {
	logger log.Logger

	db       *sql.DB
	dsn      string
	database string
	table    string

	ignoredSamples prometheus.Counter
	commitFail     prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(logger log.Logger, dsn string, database string, table string) *Client {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		level.Error(logger).Log("connecting to clickhouse", err)
		os.Exit(1)
	}

	if err := db.Ping(); err != nil {
		level.Error(logger).Log("clickhouse ping", err)
		os.Exit(1)
	}

	return &Client{
		logger:   logger,
		db:       db,
		database: database,
		table:    table,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_clickhouse_ignored_samples_total",
				Help: "The total number of samples not sent to Clickhouse due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
		commitFail: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_clickhouse_commit_fail_samples_total",
				Help: "The total number of samples not sent to Clickhouse due exec.",
			},
		),
	}
}

// Write sends a batch of samples to InfluxDB via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	tx, err := c.db.Begin()
	if err != nil {
		level.Error(c.logger).Log("begin transaction:", err)
		return err
	}

	smt, err := tx.Prepare(formatInsert(c.database, c.table))
	if err != nil {
		level.Error(c.logger).Log("prepare statement:", err)
		return err
	}

	for _, s := range samples {
		m := tagValue(s.Metric)
		ts := s.Timestamp.Time()

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			c.ignoredSamples.Inc()
			continue
		}

		_, err = smt.Exec(ts, m.metricName(), m.tagsFromMetric(), v, ts)

		if err != nil {
			level.Error(c.logger).Log("statement exec:", err)
			c.ignoredSamples.Inc()
			continue
		}
	}

	if err = tx.Commit(); err != nil {
		level.Error(c.logger).Log("commit failed:", err)
		c.commitFail.Inc()
		return err
	}

	return nil
}

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	var err error
	var rcount int
	var sqlStr string
	var rows *sql.Rows
	var sqlBuilder *sqlBuilder
	var tsres = make(map[string]*prompb.TimeSeries)

	for _, q := range req.Queries {
		sqlBuilder, err = builder(q, c.database, c.table)
		if err != nil {
			level.Error(c.logger).Log("reader getSQL", err)
			return nil, err
		}

		rows, err = c.db.Query(sqlBuilder.String())
		if err != nil {
			level.Error(c.logger).Log("reader query failed", sqlStr)
			level.Error(c.logger).Log("reader query errors", err)
			return nil, err
		}

		// build map of timeseries from sql result

		for rows.Next() {
			rcount++
			var (
				cnt   int
				t     int64
				name  string
				tags  []string
				value float64
			)
			if err = rows.Scan(&cnt, &t, &name, &tags, &value); err != nil {
				level.Error(c.logger).Log("reader scan errors", err)
			}

			// borrowed from influx remote storage adapter - array sep
			key := strings.Join(tags, "\xff")
			ts, ok := tsres[key]
			if !ok {
				ts = &prompb.TimeSeries{
					Labels: makeLabels(tags),
				}
				tsres[key] = ts
			}
			ts.Samples = append(ts.Samples, prompb.Sample{
				Value:     float64(value),
				Timestamp: t,
			})
		}
	}

	resp := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{Timeseries: make([]*prompb.TimeSeries, 0, len(tsres))},
		},
	}

	// now add results to response
	for _, ts := range tsres {
		resp.Results[0].Timeseries = append(resp.Results[0].Timeseries, ts)
	}

	return &resp, nil
}

// Name identifies the client as an InfluxDB client.
func (c Client) Name() string {
	return "clickhouse"
}

// Describe implements prometheus.Collector.
func (c *Client) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ignoredSamples.Desc()
	ch <- c.commitFail.Desc()
}

// Collect implements prometheus.Collector.
func (c *Client) Collect(ch chan<- prometheus.Metric) {
	ch <- c.ignoredSamples
	ch <- c.commitFail
}
