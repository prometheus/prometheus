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

package clickhouse

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/prometheus/prometheus/prompb"
)

type scanner struct {
	f float64
	s string
}

func (s *scanner) Scan(v interface{}) error {
	s.f = 0
	s.s = ""

	s.s = fmt.Sprintf("%v", v)
	switch v := v.(type) {
	case int64:
		s.f = float64(v)
	case uint64:
		s.f = float64(v)
	case float64:
		s.f = v
	case []byte:
		s.s = fmt.Sprintf("%s", v)
	}
	return nil
}

func (ch *clickHouse) readRawSQL(ctx context.Context, query string, ts int64) (*prompb.QueryResult, error) {
	rows, err := ch.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var res prompb.QueryResult
	targets := make([]interface{}, len(columns))
	for i := range targets {
		targets[i] = new(scanner)
	}

	for rows.Next() {
		if err = rows.Scan(targets...); err != nil {
			return nil, err
		}

		labels := make([]prompb.Label, 0, len(columns))
		var value float64
		for i, c := range columns {
			v := targets[i].(*scanner)
			switch c {
			case "value":
				value = v.f
			default:
				labels = append(labels, prompb.Label{
					Name:  c,
					Value: v.s,
				})
			}
		}

		res.Timeseries = append(res.Timeseries, &prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{{
				Value:     value,
				Timestamp: ts,
			}},
		})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &res, nil
	// return &prompb.ReadResponse{
	// 	Results: []*prompb.QueryResult{&res},
	// }, nil
}

// check interfaces
var (
	_ sql.Scanner = (*scanner)(nil)
)
