package series

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestSeries_String(t *testing.T) {
	s := Series{
		Name:   "my_metric_seconds_total",
		Unit:   "seconds",
		Type:   model.MetricTypeCounter,
		Labels: labels.FromStrings("foo", "bar"),
	}
	require.Equal(t, "my_metric_seconds_total{foo=\"bar\"}~seconds.counter", s.String())

	// UTF-8
	s2 := Series{
		Name:   "my.metric.seconds_total",
		Unit:   "seconds",
		Type:   model.MetricTypeCounter,
		Labels: labels.FromStrings("foo", "bar"),
	}
	require.Equal(t, "{\"my.metric.seconds_total\", foo=\"bar\"}~seconds.counter", s2.String())
}
