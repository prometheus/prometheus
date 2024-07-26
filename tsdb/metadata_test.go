package tsdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestDBAppenderSeriesWithMetadata(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	// Add a sample with metadata.
	_, err := app1.Append(0, labels.FromStrings(
		"__name__", "http_requests_total",
		"job", "ingester",
		"metadata.foo.service", "ingester",
		"metadata.foo.request_type", "outbound",
		"metadata.node.ip", "192.168.1.1",
	), 0, 0)
	require.NoError(t, err)

	// Add a sample with the same metadata but different value.
	_, err = app1.Append(0, labels.FromStrings(
		"__name__", "http_requests_total",
		"job", "ingester",
		"metadata.foo.service", "ingester",
		"metadata.foo.request_type", "outbound",
		"metadata.node.ip", "192.168.1.2",
	), 1, 1)
	require.NoError(t, err)

	err = app1.Commit()
	require.NoError(t, err)

	q, err := db.Querier(0, 200)
	require.NoError(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total"))

	// TODO(jesusvazquez) There should be a single series http_requests_total{job="ingester"} with two samples since the metadata does not form part of the series.
	require.Equal(t, map[string][]chunks.Sample{
		labels.FromStrings(
			"__name__", "http_requests_total",
			"job", "ingester",
		).String(): {
			sample{t: 0, f: 0},
			sample{t: 1, f: 1},
		},
	}, res)

	// TODO(jesusvazquez) Add a test to verify that the metadata is stored in the metadata store.
	// TODO(jesusvazquez) Add a test to make sure metadata changes between timestamps.
}
