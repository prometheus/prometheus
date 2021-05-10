package clickhouse

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

const (
	maxSample int64   = 8192 // Maximum number of samples to return to Prometheus for a remote read
	period    int64   = 10   // The minimum time range for Clickhouse time aggregation in seconds
	quantile  float64 = 0.75 // 拉取时间范围过大的时候 打点越稀疏 按quantile 聚合返回数据集
	insertSQL string  = `INSERT INTO %s.%s (date, name, tags, val, ts) VALUES (?, ?, ?, ?, ?)`
	selectSQL string  = "SELECT COUNT() AS CNT, (intDiv(toUInt32(ts), %d) * %d) * 1000 as t, name, tags, quantile(%f)(val) as value FROM %s.%s"
	whereSQL  string  = " WHERE date >= toDate(%d) AND ts >= toDateTime(%d) AND ts <= toDateTime(%d) "
)

func formatInsert(database string, table string) string {
	return fmt.Sprintf(insertSQL, database, table)
}

type sqlBuilder struct {
	sqlStr   bytes.Buffer
	query    *prompb.Query
	database string
	table    string
}

func builder(q *prompb.Query, database, table string) (*sqlBuilder, error) {
	var err error
	var b = new(sqlBuilder)
	b.query = q
	b.database = database
	b.table = table

	if err = b.formatSelect(); err != nil {
		return b, err
	}

	err = b.formatQuery()
	return b, err
}

func (s *sqlBuilder) formatSelect() error {
	var divStep int64
	sTime := s.query.StartTimestampMs / 1000
	eTime := s.query.EndTimestampMs / 1000

	if eTime < sTime {
		return errors.New("Start time is after end time")
	}

	if divStep = (sTime - eTime) / maxSample; divStep < period {
		divStep = period
	}

	s.sqlStr.WriteString(fmt.Sprintf(selectSQL, divStep, divStep, quantile, s.database, s.table))
	s.formatWhere(sTime, eTime)
	return nil
}

func (s *sqlBuilder) formatWhere(sTime, eTime int64) error {
	s.sqlStr.WriteString(fmt.Sprintf(whereSQL, sTime, sTime, eTime))
	return nil
}

func (s *sqlBuilder) formatQuery() error {
	for _, m := range s.query.Matchers {
		// metric name (make sure add name to index )
		if m.Name == model.MetricNameLabel {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				s.sqlStr.WriteString(fmt.Sprintf(` AND name='%s' `, strings.Replace(m.Value, `'`, `\'`, -1)))
			case prompb.LabelMatcher_NEQ:
				s.sqlStr.WriteString(fmt.Sprintf(` AND name!='%s' `, strings.Replace(m.Value, `'`, `\'`, -1)))
			case prompb.LabelMatcher_RE:
				s.sqlStr.WriteString(fmt.Sprintf(` AND match(name, %s) = 1 `, strings.Replace(m.Value, `/`, `\/`, -1)))
			case prompb.LabelMatcher_NRE:
				s.sqlStr.WriteString(fmt.Sprintf(` AND match(name, %s) = 0 `, strings.Replace(m.Value, `/`, `\/`, -1)))
			}
			continue
		}

		if m.Value == "" {
			m.Value = "''"
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ, prompb.LabelMatcher_NEQ:
			var tagFormat string
			var tags []string
			if m.Type == prompb.LabelMatcher_EQ {
				tagFormat = " AND arrayExists(x -> x IN (%s), tags) = 1 "
			} else {
				tagFormat = " AND arrayExists(x -> x IN (%s), tags) = 0 "
			}

			for _, val := range strings.Split(m.Value, "|") {
				tags = append(tags, fmt.Sprintf(`'%s=%s' `, m.Name, strings.Replace(val, `'`, `\'`, -1)))
			}

			s.sqlStr.WriteString(fmt.Sprintf(tagFormat, strings.Join(tags, " , ")))
		case prompb.LabelMatcher_RE, prompb.LabelMatcher_NRE:
			var tagFormat string
			var val string
			if m.Type == prompb.LabelMatcher_EQ {
				tagFormat = ` AND arrayExists(x -> 1 == match(x, '^%s=%s'),tags) = 1 `
			} else {
				tagFormat = ` AND arrayExists(x -> 1 == match(x, '^%s=%s'),tags) = 0 `
			}

			if strings.HasPrefix(m.Value, "^") {
				val = strings.Replace(m.Value, "^", "", 1)
			}
			val = strings.Replace(val, `/`, `\/`, -1)
			s.sqlStr.WriteString(fmt.Sprintf(tagFormat, m.Name, val))
		}
	}

	s.sqlStr.WriteString(" GROUP BY t, name, tags ORDER BY t ")
	return nil
}

func (s *sqlBuilder) String() string {
	return s.sqlStr.String()
}
