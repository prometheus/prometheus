// Copyright 2017, 2018 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package clickhouse provides ClickHouse storage.
package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2" // register SQL driver

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/base"
)

const (
	subsystem                     = "clickhouse"
	DISTRIBUTED_TIME_SERIES_TABLE = "distributed_time_series_v2"
	DISTRIBUTED_SAMPLES_TABLE     = "distributed_samples_v2"
)

// clickHouse implements storage interface for the ClickHouse.
type clickHouse struct {
	db                   *sql.DB
	l                    *logrus.Entry
	database             string
	maxTimeSeriesInQuery int

	timeSeriesRW sync.RWMutex
	// map of [metirc_name][fingerprint][labels]
	timeSeries          map[string]map[uint64][]prompb.Label
	lastLoadedTimeStamp int64
	useClickHouseQuery  bool
}

type ClickHouseParams struct {
	DSN                  string
	DropDatabase         bool
	MaxOpenConns         int
	MaxTimeSeriesInQuery int
}

func NewClickHouse(params *ClickHouseParams) (base.Storage, error) {
	l := logrus.WithField("component", "clickhouse")

	dsnURL, err := url.Parse(params.DSN)
	if err != nil {
		return nil, err
	}
	database := dsnURL.Query().Get("database")
	if database == "" {
		return nil, fmt.Errorf("database should be set in ClickHouse DSN")
	}

	options := &clickhouse.Options{
		Addr: []string{dsnURL.Host},
	}
	if dsnURL.Query().Get("username") != "" {
		auth := clickhouse.Auth{
			// Database: "",
			Username: dsnURL.Query().Get("username"),
			Password: dsnURL.Query().Get("password"),
		}

		options.Auth = auth
	}
	initDB := clickhouse.OpenDB(options)

	initDB.SetConnMaxIdleTime(2)
	initDB.SetMaxOpenConns(params.MaxOpenConns)
	initDB.SetConnMaxLifetime(0)

	if err != nil {
		return nil, fmt.Errorf("could not connect to clickhouse: %s", err)
	}

	var useClickHouseQuery bool
	if strings.ToLower(strings.TrimSpace(os.Getenv("USE_CLICKHOUSE_QUERY"))) == "true" {
		useClickHouseQuery = true
	}

	ch := &clickHouse{
		db:                   initDB,
		l:                    l,
		database:             database,
		maxTimeSeriesInQuery: params.MaxTimeSeriesInQuery,

		timeSeries:         make(map[string]map[uint64][]prompb.Label, 262144),
		useClickHouseQuery: useClickHouseQuery,
	}

	go func() {
		ctx := pprof.WithLabels(context.TODO(), pprof.Labels("component", "clickhouse_reloader"))
		pprof.SetGoroutineLabels(ctx)
		// ch.runTimeSeriesReloader(ctx)
	}()

	return ch, nil
}

func (ch *clickHouse) runTimeSeriesReloader(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	queryTmpl := `SELECT DISTINCT fingerprint, labels FROM %s.%s WHERE timestamp_ms >= $1;`
	for {
		timeSeries := make(map[string]map[uint64][]prompb.Label)
		newSeriesCount := 0
		lastLoadedTimeStamp := time.Now().Add(time.Duration(-1) * time.Minute).UnixMilli()

		err := func() error {
			query := fmt.Sprintf(queryTmpl, ch.database, DISTRIBUTED_TIME_SERIES_TABLE)
			ch.l.Debug("Running reloader query:", query)
			rows, err := ch.db.Query(query, ch.lastLoadedTimeStamp)
			if err != nil {
				return err
			}
			defer rows.Close()

			var f uint64
			var b []byte
			for rows.Next() {
				if err = rows.Scan(&f, &b); err != nil {
					return err
				}
				labels, metricName, err := unmarshalLabels(b)
				if err != nil {
					return err
				}
				if _, ok := timeSeries[metricName]; !ok {
					timeSeries[metricName] = make(map[uint64][]prompb.Label)
				}
				if _, ok := timeSeries[metricName][f]; !ok {
					timeSeries[metricName][f] = make([]prompb.Label, 0)
				}
				timeSeries[metricName][f] = labels
				newSeriesCount += 1
			}
			return rows.Err()
		}()
		if err == nil {
			ch.timeSeriesRW.Lock()
			for metricName, fingerprintsMap := range timeSeries {
				for fingerprint, labels := range fingerprintsMap {
					if _, ok := ch.timeSeries[metricName]; !ok {
						ch.timeSeries[metricName] = make(map[uint64][]prompb.Label)
					}
					ch.timeSeries[metricName][fingerprint] = labels
				}
			}
			ch.lastLoadedTimeStamp = lastLoadedTimeStamp
			ch.timeSeriesRW.Unlock()
			ch.l.Debugf("Loaded %d new time series", newSeriesCount)
		} else {
			ch.l.Error(err)
		}

		select {
		case <-ctx.Done():
			ch.l.Warn(ctx.Err())
			return
		case <-ticker.C:
		}
	}
}

func (ch *clickHouse) scanSamples(rows *sql.Rows, fingerprints map[uint64][]prompb.Label) ([]*prompb.TimeSeries, error) {
	// scan results
	var res []*prompb.TimeSeries
	var ts *prompb.TimeSeries
	var fingerprint, prevFingerprint uint64
	var timestampMs int64
	var value float64
	var metricName string

	totalFingerprintCount := len(fingerprints)
	usedFingerprintCount := 0

	for rows.Next() {
		if err := rows.Scan(&metricName, &fingerprint, &timestampMs, &value); err != nil {
			return nil, errors.WithStack(err)
		}

		// collect samples in time series
		if fingerprint != prevFingerprint {
			// add collected time series to result
			prevFingerprint = fingerprint
			if ts != nil {
				res = append(res, ts)
			}

			labels := fingerprints[fingerprint]
			ts = &prompb.TimeSeries{
				Labels: labels,
			}
			usedFingerprintCount += 1
		}

		// add samples to current time series
		ts.Samples = append(ts.Samples, prompb.Sample{
			Timestamp: timestampMs,
			Value:     value,
		})
	}

	// add last time series
	if ts != nil {
		res = append(res, ts)
	}

	ch.l.Infof("Used %d time series from %d time series returned by query", usedFingerprintCount, totalFingerprintCount)

	if err := rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}

func (ch *clickHouse) querySamples(
	ctx context.Context,
	start int64,
	end int64,
	fingerprints map[uint64][]prompb.Label,
	metricName string,
	subQuery string,
	args []interface{},
) ([]*prompb.TimeSeries, error) {

	argCount := len(args)

	query := fmt.Sprintf(`
		SELECT metric_name, fingerprint, timestamp_ms, value
			FROM %s.%s
			WHERE metric_name = $1 AND fingerprint GLOBAL IN (%s) AND timestamp_ms >= $%s AND timestamp_ms <= $%s ORDER BY fingerprint, timestamp_ms;`,
		ch.database, DISTRIBUTED_SAMPLES_TABLE, subQuery, strconv.Itoa(argCount+2), strconv.Itoa(argCount+3))
	query = strings.TrimSpace(query)

	ch.l.Debugf("Running query : %s", query)

	allArgs := append([]interface{}{metricName}, args...)
	allArgs = append(allArgs, start, end)

	// run query
	rows, err := ch.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	return ch.scanSamples(rows, fingerprints)
}

// convertReadRequest converts protobuf read request into a slice of storage queries.
func (ch *clickHouse) convertReadRequest(request *prompb.Query) base.Query {

	q := base.Query{
		Start:    model.Time(request.StartTimestampMs),
		End:      model.Time(request.EndTimestampMs),
		Matchers: make([]base.Matcher, len(request.Matchers)),
	}

	for j, m := range request.Matchers {
		var t base.MatchType
		switch m.Type {
		case prompb.LabelMatcher_EQ:
			t = base.MatchEqual
		case prompb.LabelMatcher_NEQ:
			t = base.MatchNotEqual
		case prompb.LabelMatcher_RE:
			t = base.MatchRegexp
		case prompb.LabelMatcher_NRE:
			t = base.MatchNotRegexp
		default:
			ch.l.Error("convertReadRequest: unexpected matcher %d", m.Type)
		}

		q.Matchers[j] = base.Matcher{
			Type:  t,
			Name:  m.Name,
			Value: m.Value,
		}
	}

	if request.Hints != nil {
		ch.l.Warnf("Ignoring hint %+v for query %v.", *request.Hints, q)
	}

	return q
}

func (ch *clickHouse) prepareClickHouseQuery(query *prompb.Query, metricName string, subQuery bool) (string, []interface{}, error) {
	var clickHouseQuery string
	var conditions []string
	var argCount int = 0
	var selectString string = "fingerprint, labels"
	if subQuery {
		argCount = 1
		selectString = "fingerprint"
	}
	var args []interface{}
	conditions = append(conditions, fmt.Sprintf("metric_name = $%d", argCount+1))
	args = append(args, metricName)
	for _, m := range query.Matchers {
		switch m.Type {
		case prompb.LabelMatcher_EQ:
			conditions = append(conditions, fmt.Sprintf("JSONExtractString(labels, $%d) = $%d", argCount+2, argCount+3))
		case prompb.LabelMatcher_NEQ:
			conditions = append(conditions, fmt.Sprintf("JSONExtractString(labels, $%d) != $%d", argCount+2, argCount+3))
		case prompb.LabelMatcher_RE:
			conditions = append(conditions, fmt.Sprintf("match(JSONExtractString(labels, $%d), $%d)", argCount+2, argCount+3))
		case prompb.LabelMatcher_NRE:
			conditions = append(conditions, fmt.Sprintf("not match(JSONExtractString(labels, $%d), $%d)", argCount+2, argCount+3))
		default:
			return "", nil, fmt.Errorf("prepareClickHouseQuery: unexpected matcher %d", m.Type)
		}
		args = append(args, m.Name, m.Value)
		argCount += 2
	}
	whereClause := strings.Join(conditions, " AND ")

	clickHouseQuery = fmt.Sprintf(`SELECT DISTINCT %s FROM %s.%s WHERE %s`, selectString, ch.database, DISTRIBUTED_TIME_SERIES_TABLE, whereClause)

	return clickHouseQuery, args, nil
}

func (ch *clickHouse) fingerprintsForQuery(ctx context.Context, query string, args []interface{}) (map[uint64][]prompb.Label, error) {
	// run query
	rows, err := ch.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// scan results
	var fingerprint uint64
	var b []byte
	fingerprints := make(map[uint64][]prompb.Label)
	for rows.Next() {
		if err = rows.Scan(&fingerprint, &b); err != nil {
			return nil, err
		}

		labels, _, err := unmarshalLabels(b)

		if err != nil {
			return nil, err
		}
		fingerprints[fingerprint] = labels
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return fingerprints, nil
}

func (ch *clickHouse) Read(ctx context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
	// special case for {{job="rawsql", query="SELECT …"}} (start is ignored)
	if len(query.Matchers) == 2 {
		var hasJob bool
		var queryString string
		for _, m := range query.Matchers {
			if base.MatchType(m.Type) == base.MatchEqual && m.Name == "job" && m.Value == "rawsql" {
				hasJob = true
			}
			if base.MatchType(m.Type) == base.MatchEqual && m.Name == "query" {
				queryString = m.Value
			}
		}
		if hasJob && queryString != "" {
			return ch.readRawSQL(ctx, queryString, int64(query.EndTimestampMs))
		}
	}

	convertedQuery := ch.convertReadRequest(query)

	var metricName string
	for _, matcher := range convertedQuery.Matchers {
		if matcher.Name == "__name__" {
			metricName = matcher.Value
		}
	}

	var fingerprints map[uint64][]prompb.Label
	var err error

	clickHouseQuery, args, err := ch.prepareClickHouseQuery(query, metricName, false)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	fingerprints, err = ch.fingerprintsForQuery(ctx, clickHouseQuery, args)
	if err != nil {
		return nil, err
	}
	ch.l.Debugf("fingerprintsForQuery took %s, num fingerprints: %d", time.Since(start), len(fingerprints))
	subQuery, args, err := ch.prepareClickHouseQuery(query, metricName, true)

	res := new(prompb.QueryResult)
	if len(fingerprints) == 0 {
		return res, nil
	}

	sampleFunc := ch.querySamples
	// if len(fingerprints) > ch.maxTimeSeriesInQuery {
	// 	sampleFunc = ch.tempTableSamples
	// }

	ts, err := sampleFunc(ctx, int64(query.StartTimestampMs), int64(query.EndTimestampMs), fingerprints, metricName, subQuery, args)

	if err != nil {
		return nil, err
	}
	res.Timeseries = ts

	return res, nil
}
