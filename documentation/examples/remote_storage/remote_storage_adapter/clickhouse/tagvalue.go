package clickhouse

import (
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type tagValue model.Metric

func (t tagValue) metricName() string {
	if metricName, ok := t[model.MetricNameLabel]; ok {
		return string(metricName)
	}
	return ""
}

// tagsFromMetric extracts Clickhouse tags from a Prometheus metric.
func (t tagValue) tagsFromMetric() []string {
	var tags []string
	for l, v := range t {
		tags = append(tags, fmt.Sprintf("%s=%s", l, v))
	}
	sort.Strings(tags)

	return tags
}

func makeLabels(tags []string) []*prompb.Label {
	lpairs := make([]*prompb.Label, 0, len(tags))
	for _, tag := range tags {
		vals := strings.SplitN(tag, "=", 2)
		if len(vals) != 2 {
			fmt.Printf("Error unpacking tag key/val: %s\n", tag)
			continue
		}
		if vals[1] == "" {
			continue
		}
		lpairs = append(lpairs, &prompb.Label{
			Name:  vals[0],
			Value: vals[1],
		})
	}
	return lpairs
}
