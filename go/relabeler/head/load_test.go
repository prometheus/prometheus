package head_test

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "head_wal_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	const transparentRelabelerName = "transparent_relabeler"

	cfgs := []*config.InputRelabelerConfig{
		{
			Name: transparentRelabelerName,
			RelabelConfigs: []*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"__name__"},
					Regex:        ".*",
					Action:       cppbridge.Keep,
				},
			},
		},
	}
	headID := "head_id"
	h, err := head.Load(headID, 0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	require.NoError(t, err)

	ls := model.NewLabelSetBuilder().Set("__name__", "wal_metric").Set("job", "test").Build()
	require.NoError(t, appendTimeSeries(t, ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
	}))

	require.NoError(t, appendTimeSeries(t, ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
	}))

	require.NoError(t, appendTimeSeries(t, ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
	}))

	q := querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	require.NoError(t, err)
	seriesSet := q.Select(ctx, false, nil, matcher)

	expected := []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
	}

	require.True(t, seriesSet.Next())
	series := seriesSet.At()

	// todo compare label sets
	chunkIterator := series.Iterator(nil)
	sIndex := 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		require.Equal(t, int64(expected[sIndex].Timestamp), ts)
		require.Equal(t, expected[sIndex].Value, v)
		sIndex++
	}
	require.Equal(t, sIndex, len(expected))

	require.False(t, seriesSet.Next())

	require.NoError(t, q.Close())

	h.Finalize()
	require.NoError(t, h.Close())

	h, err = head.Load(headID, 0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	require.NoError(t, err)

	q = querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err = labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	require.NoError(t, err)
	seriesSet = q.Select(ctx, false, nil, matcher)

	expected = []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
	}

	require.True(t, seriesSet.Next())
	series = seriesSet.At()

	// todo compare label sets
	chunkIterator = series.Iterator(nil)
	sIndex = 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		require.Equal(t, int64(expected[sIndex].Timestamp), ts)
		require.Equal(t, expected[sIndex].Value, v)
		sIndex++
	}
	require.Equal(t, sIndex, len(expected))

	require.False(t, seriesSet.Next())

	require.NoError(t, q.Close())

	require.NoError(t, appendTimeSeries(t, ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 3,
			Value:     4,
		},
	}))

	q = querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err = labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	require.NoError(t, err)
	seriesSet = q.Select(ctx, false, nil, matcher)

	expected = []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
		{
			LabelSet:  ls,
			Timestamp: 3,
			Value:     4,
		},
	}

	require.True(t, seriesSet.Next())
	series = seriesSet.At()

	// todo compare label sets
	chunkIterator = series.Iterator(nil)
	sIndex = 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		require.Equal(t, int64(expected[sIndex].Timestamp), ts)
		require.Equal(t, expected[sIndex].Value, v)
		sIndex++
	}
	require.Equal(t, sIndex, len(expected))

	require.False(t, seriesSet.Next())

	require.NoError(t, q.Close())

	h.Finalize()
	require.NoError(t, h.Close())

	h, err = head.Load(headID, 0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	require.NoError(t, err)

	q = querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err = labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	require.NoError(t, err)
	seriesSet = q.Select(ctx, false, nil, matcher)

	expected = []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
		{
			LabelSet:  ls,
			Timestamp: 3,
			Value:     4,
		},
	}

	require.True(t, seriesSet.Next())
	series = seriesSet.At()

	// todo compare label sets
	chunkIterator = series.Iterator(nil)
	sIndex = 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		require.Equal(t, int64(expected[sIndex].Timestamp), ts)
		require.Equal(t, expected[sIndex].Value, v)
		sIndex++
	}
	require.Equal(t, sIndex, len(expected))

	require.False(t, seriesSet.Next())

	require.NoError(t, q.Close())

	require.NoError(t, appendTimeSeries(t, ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 4,
			Value:     5,
		},
	}))

	q = querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err = labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	require.NoError(t, err)
	seriesSet = q.Select(ctx, false, nil, matcher)

	expected = []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
		{
			LabelSet:  ls,
			Timestamp: 3,
			Value:     4,
		},
		{
			LabelSet:  ls,
			Timestamp: 4,
			Value:     5,
		},
	}

	require.True(t, seriesSet.Next())
	series = seriesSet.At()

	// todo compare label sets
	chunkIterator = series.Iterator(nil)
	sIndex = 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		require.Equal(t, int64(expected[sIndex].Timestamp), ts)
		require.Equal(t, expected[sIndex].Value, v)
		sIndex++
	}
	require.Equal(t, sIndex, len(expected))

	require.False(t, seriesSet.Next())

	require.NoError(t, q.Close())

	h.Finalize()
	require.NoError(t, h.Close())
}

type timeSeriesData struct {
	timeSeries []model.TimeSeries
}

func (tsd *timeSeriesData) TimeSeries() []model.TimeSeries {
	return tsd.timeSeries
}

func (tsd *timeSeriesData) Destroy() {
	tsd.timeSeries = nil
}

func appendTimeSeries(t *testing.T, ctx context.Context, h *head.Head, timeSeries []model.TimeSeries) error {
	tsd := &timeSeriesData{timeSeries: timeSeries}
	hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
	require.NoError(t, err)

	incomingData := &relabeler.IncomingData{Hashdex: hx, Data: tsd}
	metricLimits := &cppbridge.MetricLimits{
		LabelLimit:            30,
		LabelNameLengthLimit:  200,
		LabelValueLengthLimit: 200,
		SampleLimit:           1500,
	}

	_, err = h.Append(ctx, incomingData, metricLimits, nil, 0, "transparent_relabeler")
	return err
}
