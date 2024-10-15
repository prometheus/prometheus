package main

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
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "head_wal_test")
	if err != nil {
		panic(err)
	}
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
	h, err := head.Load(0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	if err != nil {
		panic(err)
	}

	ls := model.NewLabelSetBuilder().Set("__name__", "wal_metric").Set("job", "test").Build()
	err = appendTimeSeries(ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 0,
			Value:     1,
		},
	})
	if err != nil {
		panic(err)
	}

	err = appendTimeSeries(ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 1,
			Value:     2,
		},
	})
	if err != nil {
		panic(err)
	}

	err = appendTimeSeries(ctx, h, []model.TimeSeries{
		{
			LabelSet:  ls,
			Timestamp: 2,
			Value:     3,
		},
	})
	if err != nil {
		panic(err)
	}

	q := querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	if err != nil {
		panic(err)
	}
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

	if !seriesSet.Next() {
		panic("no series")
	}

	series := seriesSet.At()

	// todo compare label sets
	chunkIterator := series.Iterator(nil)
	sIndex := 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		if int64(expected[sIndex].Timestamp) != ts {
			panic("int64(expected[sIndex].Timestamp) != ts ")
		}
		if expected[sIndex].Value != v {
			panic("expected[sIndex].Value != v ")
		}
		sIndex++
	}
	if sIndex != len(expected) {
		panic("sIndex != len(expected)")
	}

	if seriesSet.Next() {
		panic("seriesSet.Next() must be false")
	}

	h.Finalize()
	if err = h.Close(); err != nil {
		panic(err)
	}

	h, err = head.Load(0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	if err != nil {
		panic(err)
	}

	q = querier.NewQuerier(h, querier.NoOpShardedDeduplicatorFactory(), 0, 10, nil, nil)
	matcher, err = labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	if err != nil {
		panic(err)
	}
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

	if !seriesSet.Next() {
		panic("!seriesSet.Next()")
	}

	series = seriesSet.At()

	// todo compare label sets
	chunkIterator = series.Iterator(nil)
	sIndex = 0
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		if int64(expected[sIndex].Timestamp) != ts {
			panic("int64(expected[sIndex].Timestamp) != ts ")
		}
		if expected[sIndex].Value != v {
			panic("expected[sIndex].Value != v ")
		}
		sIndex++
	}
	if sIndex != len(expected) {
		panic("sIndex != len(expected)")
	}

	if seriesSet.Next() {
		panic("seriesSet.Next() must be false")
	}

	h.Finalize()
	if err = h.Close(); err != nil {
		panic(err)
	}
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

func appendTimeSeries(ctx context.Context, h *head.Head, timeSeries []model.TimeSeries) error {
	tsd := &timeSeriesData{timeSeries: timeSeries}
	hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
	if err != nil {
		return err
	}

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
