package appender_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/appender"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/distributor"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
)

const finalFrame = "final"

type AppenderSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestAppender(t *testing.T) {
	suite.Run(t, new(AppenderSuite))
}

func (s *AppenderSuite) SetupSuite() {
	s.baseCtx = context.Background()
	s.errorHandler()
}

func (s *AppenderSuite) TestManagerRelabelerKeep() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)

	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()

	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.2, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabeling() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"})
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingAddNewLabel() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithExternalLabelsEnd() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	externalLabels := []cppbridge.Label{{Name: "zzz", Value: "zzz"}}

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		externalLabels,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		externalLabels,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.Require().NoError(err)
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)
	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"}, prompb.Label{Name: "zzz", Value: "zzz"})
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"}, prompb.Label{Name: "zzz", Value: "zzz"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithExternalLabelsRelabel() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	externalLabels := []cppbridge.Label{{Name: "job", Value: "zzz"}}

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		externalLabels,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		externalLabels,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"})
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "job2", Value: "boj"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithTargetLabels() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	options := &cppbridge.RelabelerOptions{
		TargetLabels: []cppbridge.Label{{Name: "zname", Value: "target_value"}},
	}
	state := cppbridge.NewState(numberOfShards)
	state.SetRelabelerOptions(options)

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
					{Name: "zname", Value: "target_value"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}

	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
					{Name: "zname", Value: "target_value"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
		},
	}
	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithTargetLabels_ConflictingLabels() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	options := &cppbridge.RelabelerOptions{
		TargetLabels: []cppbridge.Label{{Name: "instance", Value: "target_instance"}},
	}
	state := cppbridge.NewState(numberOfShards)
	state.SetRelabelerOptions(options)

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "exported_instance", Value: "value1"},
					{Name: "instance", Value: "target_instance"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}

	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "exported_instance", Value: "value1"},
					{Name: "instance", Value: "target_instance"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
		},
	}
	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithTargetLabels_ConflictingLabels_Honor() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	options := &cppbridge.RelabelerOptions{
		TargetLabels: []cppbridge.Label{{Name: "instance", Value: "target_instance"}},
		HonorLabels:  true,
	}
	state := cppbridge.NewState(numberOfShards)
	state.SetRelabelerOptions(options)

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        "some:([^-]+):([^,]+)",
			TargetLabel:  "${1}",
			Replacement:  "${2}",
			Action:       cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}

	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	expectedWr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "job2", Value: "boj"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
		},
	}
	s.Equal(expectedWr.String(), <-destination)
	s.Equal(expectedWr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

type noOpStorage struct{}

func (noOpStorage) Add(_ relabeler.Head) {}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithRotate() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	var tmpDirsToRemove []string
	defer func() {
		for _, tmpDirToRemove := range tmpDirsToRemove {
			_ = os.RemoveAll(tmpDirToRemove)
		}
	}()

	var headsToClose []io.Closer
	defer func() {
		for _, h := range headsToClose {
			_ = h.Close()
		}
	}()
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	tmpDirsToRemove = append(tmpDirsToRemove, tmpDir)

	var generation uint64 = 0

	headID := "head_id"
	hd, err := head.Create(headID, generation, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	headsToClose = append(headsToClose, hd)

	builder := head.NewBuilder(
		head.ConfigSourceFunc(
			func() ([]*config.InputRelabelerConfig, uint16) {
				return inputRelabelerConfigs, numberOfShards
			},
		),
		func(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error) {
			newDir, buildErr := os.MkdirTemp("", "appender_test")
			s.Require().NoError(buildErr)
			tmpDirsToRemove = append(tmpDirsToRemove, newDir)
			newHeadID := "head_id"
			generation++
			newHead, buildErr := head.Create(newHeadID, generation, newDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
			s.Require().NoError(buildErr)
			headsToClose = append(headsToClose, newHead)
			return newHead, nil
		},
	)

	rotatableHead := appender.NewRotatableHead(hd, noOpStorage{}, builder, appender.NoOpHeadActivator{})
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(rotatableHead, dstrb, metrics)

	rotationTimer := relabeler.NewRotateTimer(clock, appender.DefaultRotateDuration)
	commitTimer := appender.NewConstantIntervalTimer(clock, appender.DefaultCommitTimeout)
	defer rotationTimer.Stop()
	rotator := appender.NewRotateCommiter(app, rotationTimer, commitTimer, prometheus.DefaultRegisterer)
	rotator.Run()
	defer func() { _ = rotator.Close() }()

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	stats, err := app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("first wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("rotate")
	clock.Advance(2 * time.Hour)
	time.Sleep(1 * time.Millisecond)

	s.T().Log("first final frame")
	for i := 0; i < 1<<relabeler.DefaultShardsNumberPower*2; i++ {
		s.Equal(finalFrame, <-destination)
	}

	s.T().Log("append second data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("second wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("append third data")
	wr = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(wr, hlimits)
	stats, err = app.Append(s.baseCtx, h, nil, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 0}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("third wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	wr.Timeseries = wr.Timeseries[:1]
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) makeDestinationGroup(
	destinationName string,
	destination chan string,
	clock clockwork.Clock,
	externalLabels []cppbridge.Label,
	relabelingCfgs []*cppbridge.RelabelConfig,
	numberOfShards uint16,
) *relabeler.DestinationGroup {
	s.T().Log("make DestinationGroupConfig")
	dir, err := s.mkDir(destinationName)
	s.Require().NoError(err)
	dgcfg := &relabeler.DestinationGroupConfig{
		Name:           destinationName,
		Dir:            dir,
		Relabeling:     relabelingCfgs,
		ExternalLabels: externalLabels,
		ManagerKeeper: &relabeler.ManagerKeeperConfig{
			ShutdownTimeout:       10 * time.Second,
			UncommittedTimeWindow: 5 * time.Second,
			RefillSenderManager: relabeler.RefillSendManagerConfig{
				ScanInterval:  3 * time.Second,
				MaxRefillSize: 10000,
			},
		},
		NumberOfShards: numberOfShards,
	}

	s.T().Log("use auto-ack transport (ack segements after ms delay), default 1 shards with 2 destination groups")
	dialers := []relabeler.Dialer{s.transportNewAutoAck(s.T().Name(), 10*time.Millisecond, destination)}

	s.T().Log("use no-op refill: assumed that it won't be touched")
	refillCtor := s.constructorForRefill(&ManagerRefillMock{
		AckFunc:                func(cppbridge.SegmentKey, string) {},
		WriteAckStatusFunc:     func(context.Context) error { return nil },
		IntermediateRenameFunc: func() error { return nil },
		ShutdownFunc:           func(context.Context) error { return nil },
	})
	refillSenderCtor := s.constructorForRefillSender(&ManagerRefillSenderMock{})

	destinationGroup, err := relabeler.NewDestinationGroup(
		s.baseCtx,
		dgcfg,
		s.encoderSelector,
		refillCtor,
		refillSenderCtor,
		clock,
		dialers,
		nil,
	)
	s.Require().NoError(err)

	return destinationGroup
}

func (s *AppenderSuite) makeIncomingData(
	wr *prompb.WriteRequest,
	hlimits cppbridge.WALHashdexLimits,
) *relabeler.IncomingData {
	data, err := wr.Marshal()
	s.Require().NoError(err)
	h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
	s.Require().NoError(err)
	// incomingData
	return &relabeler.IncomingData{Hashdex: h, Data: newProtoDataTest(data)}
}

// protoDataTest - test data.
type protoDataTest struct {
	data []byte
}

func newProtoDataTest(data []byte) *protoDataTest {
	return &protoDataTest{
		data: data,
	}
}

// Bytes - return bytes, for implements.
func (pd *protoDataTest) Bytes() []byte {
	return pd.data
}

// Destroy - clear memory, for implements.
func (pd *protoDataTest) Destroy() {
	pd.data = nil
}

func (*AppenderSuite) constructorForRefill(refill *ManagerRefillMock) relabeler.ManagerRefillCtor {
	return func(
		_ string,
		blockID uuid.UUID,
		destinations []string,
		shardsNumberPower, _ uint8,
		_ bool,
		_ string,
		_ prometheus.Registerer,
	) (relabeler.ManagerRefill, error) {
		if refill.BlockIDFunc == nil {
			refill.BlockIDFunc = func() uuid.UUID { return blockID }
		}
		refill.ShardsFunc = func() int { return 1 << shardsNumberPower }
		if refill.DestinationsFunc == nil {
			refill.DestinationsFunc = func() int { return len(destinations) }
		}
		if refill.LastSegmentFunc == nil {
			refill.LastSegmentFunc = func(uint16, string) uint32 { return math.MaxUint32 }
		}
		if refill.IsContinuableFunc == nil {
			refill.IsContinuableFunc = func() bool { return true }
		}

		return refill, nil
	}
}

func (*AppenderSuite) constructorForRefillSender(mrs *ManagerRefillSenderMock) relabeler.MangerRefillSenderCtor {
	return func(
		_ relabeler.RefillSendManagerConfig,
		_ string,
		_ []relabeler.Dialer,
		_ clockwork.Clock,
		_ string,
		_ prometheus.Registerer,
	) (relabeler.ManagerRefillSender, error) {
		if mrs.RunFunc == nil {
			mrs.RunFunc = func(ctx context.Context) {
				<-ctx.Done()
				if !errors.Is(context.Cause(ctx), relabeler.ErrShutdown) {
					relabeler.Errorf("scan and send loop context canceled: %s", context.Cause(ctx))
				}
			}
		}
		if mrs.ShutdownFunc == nil {
			mrs.ShutdownFunc = func(ctx context.Context) error {
				if ctx.Err() != nil && !errors.Is(context.Cause(ctx), relabeler.ErrShutdown) {
					relabeler.Errorf("scan and send loop context canceled: %s", context.Cause(ctx))
					return context.Cause(ctx)
				}

				return nil
			}
		}
		return mrs, nil
	}
}

func (s *AppenderSuite) errorHandler() {
	relabeler.Errorf = s.T().Errorf
	relabeler.Warnf = s.T().Logf
	relabeler.Infof = s.T().Logf
	relabeler.Debugf = s.T().Logf

	logger.Errorf = s.T().Errorf
	logger.Warnf = s.T().Logf
	logger.Infof = s.T().Logf
	logger.Debugf = s.T().Logf
}

func (*AppenderSuite) mkDir(dName string) (string, error) {
	return os.MkdirTemp("", filepath.Clean(fmt.Sprintf("refill-%s-", dName)))
}

// revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (*AppenderSuite) encoderSelector(isShrinkable bool) relabeler.ManagerEncoderCtor {
	if isShrinkable {
		return func(shardID uint16, shardsNumberPower uint8) relabeler.ManagerEncoder {
			return cppbridge.NewWALEncoderLightweight(shardID, shardsNumberPower)
		}
	}

	return func(shardID uint16, shardsNumberPower uint8) relabeler.ManagerEncoder {
		return cppbridge.NewWALEncoder(shardID, shardsNumberPower)
	}
}

//revive:disable-next-line:cognitive-complexity this is test
func (*AppenderSuite) transportNewAutoAck(
	name string,
	delay time.Duration,
	dest chan string,
) relabeler.Dialer {
	return &DialerMock{
		StringFunc: func() string { return name },
		DialFunc: func(_ context.Context, _ relabeler.ShardMeta) (relabeler.Transport, error) {
			m := new(sync.Mutex)
			var ack func(uint32)
			decoder := cppbridge.NewWALDecoder(3)
			transport := &TransportMock{
				OnAckFunc: func(fn func(uint32)) {
					m.Lock()
					ack = fn
					m.Unlock()
				},
				OnRejectFunc:    func(_ func(uint32)) {},
				OnReadErrorFunc: func(_ func(error)) {},
				SendFunc: func(ctx context.Context, frame frames.FrameWriter) error {
					rs, err := framestest.ReadSegment(ctx, frame)
					if err != nil {
						return err
					}

					if rs.GetSize() == 0 {
						// Final
						dest <- finalFrame
						return nil
					}

					pc, err := decoder.Decode(ctx, rs.Body)
					if err != nil {
						panic(err)
					}
					wr := &prompb.WriteRequest{}
					err = pc.UnmarshalTo(wr)
					if err != nil {
						return err
					}

					if wr.String() == "" {
						m.Lock()
						ack(rs.ID)
						m.Unlock()
						return nil
					}

					time.AfterFunc(delay, func() {
						m.Lock()
						ack(rs.ID)
						select {
						case dest <- wr.String():
						default:
						}
						m.Unlock()
					})
					return nil
				},
				ListenFunc: func(_ context.Context) {},
				CloseFunc: func() error {
					m.Lock()
					defer m.Unlock()
					ack = func(_ uint32) {}
					return nil
				},
			}

			return transport, nil
		},
	}
}

func (s *AppenderSuite) TestManagerRelabelerKeepWithStaleNans() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	firstWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h := s.makeIncomingData(firstWr, hlimits)

	state := cppbridge.NewState(numberOfShards)
	state.EnableTrackStaleness()
	state.SetDefTimestamp(time.Now().UnixMilli() + 1000)

	stats, err := app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	firstWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	s.Equal(firstWr.String(), <-destination)
	s.Equal(firstWr.String(), <-destination)

	s.T().Log("append second data")
	secondWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value3"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.2, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}

	h = s.makeIncomingData(secondWr, hlimits)
	state.SetDefTimestamp(time.Now().UnixMilli() + 1000)
	stats, err = app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	secondWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	firstWr.Timeseries = append(firstWr.Timeseries, secondWr.Timeseries...)
	firstWr.Timeseries[0].Samples[0].Value = math.NaN()
	firstWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()

	s.Equal(firstWr.String(), <-destination)
	s.Equal(firstWr.String(), <-destination)

	s.T().Log("shutdown distributor")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerKeepWithStaleNans_WithNullTimestamp() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	firstWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: cppbridge.NullTimestamp},
				},
			},
		},
	}
	h := s.makeIncomingData(firstWr, hlimits)

	state := cppbridge.NewState(numberOfShards)
	state.EnableTrackStaleness()
	state.SetDefTimestamp(time.Now().UnixMilli())

	stats, err := app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	firstWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	s.Equal(firstWr.String(), <-destination)
	s.Equal(firstWr.String(), <-destination)

	s.T().Log("append second data")
	secondTS := time.Now().UnixMilli()
	secondWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value3"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.2, Timestamp: secondTS},
				},
			},
		},
	}

	h = s.makeIncomingData(secondWr, hlimits)
	state.SetDefTimestamp(secondTS)
	stats, err = app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	firstWr.Timeseries = append(firstWr.Timeseries, secondWr.Timeseries...)
	firstWr.Timeseries[0].Samples[0].Value = math.NaN()
	firstWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()

	s.Equal(firstWr.String(), <-destination)
	s.Equal(firstWr.String(), <-destination)

	s.T().Log("shutdown distributor")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerKeepWithStaleNans_HonorTimestamps() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{{SourceLabels: []string{"job"}, Regex: "abc", Action: cppbridge.Keep}},
		),
	}

	destinationGroups := relabeler.DestinationGroups{destinationGroup1, destinationGroup2}

	dstrb := distributor.NewDistributor(destinationGroups)
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	headID := "head_id"
	hd, err := head.Create(headID, 0, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	defer func() { _ = hd.Close() }()
	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(hd, dstrb, metrics)

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	firstWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{{Value: 0.1, Timestamp: 720000000}},
			},
		},
	}
	h := s.makeIncomingData(firstWr, hlimits)

	state := cppbridge.NewState(numberOfShards)
	state.EnableTrackStaleness()
	state.SetDefTimestamp(time.Now().UnixMilli())
	state.SetRelabelerOptions(&cppbridge.RelabelerOptions{HonorTimestamps: true})
	stats, err := app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	s.Equal(firstWr.String(), <-destination)
	s.Equal(firstWr.String(), <-destination)

	s.T().Log("append second data")
	secondWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value3"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{{Value: 0.2, Timestamp: time.Now().UnixMilli()}},
			},
		},
	}

	h = s.makeIncomingData(secondWr, hlimits)
	state.SetDefTimestamp(time.Now().UnixMilli())
	stats, err = app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("wait send to 2 destinations")
	s.Equal(secondWr.String(), <-destination)
	s.Equal(secondWr.String(), <-destination)

	s.T().Log("shutdown distributor")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}

func (s *AppenderSuite) TestManagerRelabelerRelabelingWithRotateWithStaleNans() {
	relabelerID := s.T().Name()
	destination := make(chan string, 16)

	clock := clockwork.NewFakeClock()
	var numberOfShards uint16 = 3

	relabelingCfgs := []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup1 := s.makeDestinationGroup(
		"destination_1",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	relabelingCfgs = []*cppbridge.RelabelConfig{
		{
			Regex:       ".*",
			TargetLabel: "lname",
			Replacement: "blabla",
			Action:      cppbridge.Replace,
		},
	}
	destinationGroup2 := s.makeDestinationGroup(
		"destination_2",
		destination,
		clock,
		nil,
		relabelingCfgs,
		numberOfShards,
	)

	s.T().Log("make input relabeler")
	inputRelabelerConfigs := []*config.InputRelabelerConfig{
		config.NewInputRelabelerConfig(
			relabelerID,
			[]*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"job"},
					Regex:        "abc",
					Action:       cppbridge.Keep,
				},
			},
		),
	}

	destinationGroups := relabeler.DestinationGroups{
		destinationGroup1,
		destinationGroup2,
	}

	dstrb := distributor.NewDistributor(destinationGroups)

	var tmpDirsToRemove []string
	defer func() {
		for _, tmpDirToRemove := range tmpDirsToRemove {
			_ = os.RemoveAll(tmpDirToRemove)
		}
	}()

	var headsToClose []io.Closer
	defer func() {
		for _, h := range headsToClose {
			_ = h.Close()
		}
	}()
	tmpDir, err := os.MkdirTemp("", "appender_test")
	s.Require().NoError(err)
	tmpDirsToRemove = append(tmpDirsToRemove, tmpDir)

	var generation uint64 = 0

	headID := fmt.Sprintf("head_id_%d", generation)
	hd, err := head.Create(headID, generation, tmpDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
	s.Require().NoError(err)
	headsToClose = append(headsToClose, hd)

	builder := head.NewBuilder(
		head.ConfigSourceFunc(
			func() ([]*config.InputRelabelerConfig, uint16) {
				return inputRelabelerConfigs, numberOfShards
			},
		),
		func(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error) {
			newDir, buildErr := os.MkdirTemp("", "appender_test")
			s.Require().NoError(buildErr)
			tmpDirsToRemove = append(tmpDirsToRemove, newDir)
			generation++
			newHeadID := fmt.Sprintf("head_id_%d", generation)
			newHead, buildErr := head.Create(newHeadID, generation, newDir, inputRelabelerConfigs, numberOfShards, 0, head.NoOpLastAppendedSegmentIDSetter{}, prometheus.DefaultRegisterer)
			s.Require().NoError(buildErr)
			headsToClose = append(headsToClose, newHead)
			return newHead, nil
		},
	)

	rotatableHead := appender.NewRotatableHead(hd, noOpStorage{}, builder, appender.NoOpHeadActivator{})

	s.T().Log("make appender")
	metrics := querier.NewMetrics(prometheus.DefaultRegisterer)
	app := appender.NewQueryableAppender(rotatableHead, dstrb, metrics)

	rotationTimer := relabeler.NewRotateTimer(clock, appender.DefaultRotateDuration)
	commitTimer := appender.NewConstantIntervalTimer(clock, appender.DefaultCommitTimeout)
	defer rotationTimer.Stop()
	rotator := appender.NewRotateCommiter(app, rotationTimer, commitTimer, prometheus.DefaultRegisterer)
	rotator.Run()
	defer func() { _ = rotator.Close() }()

	hlimits := cppbridge.DefaultWALHashdexLimits()

	s.T().Log("append first data")
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value1"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 100},
				},
			},
		},
	}
	h := s.makeIncomingData(wr, hlimits)
	state := cppbridge.NewState(numberOfShards)
	state.EnableTrackStaleness()
	state.SetDefTimestamp(time.Now().UnixMilli())
	stats, err := app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel := context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("first wait send to 2 destinations")
	wr.Timeseries[0].Labels = append(wr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	wr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	s.Equal(wr.String(), <-destination)
	s.Equal(wr.String(), <-destination)

	s.T().Log("rotate")
	clock.Advance(2 * time.Hour)
	time.Sleep(1 * time.Millisecond)

	s.T().Log("first final frame")
	for i := 0; i < 1<<relabeler.DefaultShardsNumberPower*2; i++ {
		s.Equal(finalFrame, <-destination)
	}

	s.T().Log("append second data")
	secondWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(secondWr, hlimits)
	state.SetDefTimestamp(time.Now().UnixMilli())
	stats, err = app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("second wait send to 2 destinations")
	secondWr.Timeseries[0].Labels = append(secondWr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	secondWr.Timeseries = secondWr.Timeseries[:1]
	secondWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	s.Equal(secondWr.String(), <-destination)
	s.Equal(secondWr.String(), <-destination)

	s.T().Log("append third data")
	thirdWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "some:job2:boj"},
					{Name: "instance", Value: "value3"},
					{Name: "job", Value: "abc"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: 101},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value2"},
					{Name: "job", Value: "abv"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	h = s.makeIncomingData(thirdWr, hlimits)
	state.SetDefTimestamp(time.Now().UnixMilli())
	stats, err = app.AppendWithStaleNans(s.baseCtx, h, state, relabelerID, true)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 1, SeriesAdded: 1}, stats)

	clockCtx, clockCancel = context.WithTimeout(s.baseCtx, 50*time.Millisecond)
	clock.BlockUntilContext(clockCtx, 4)
	clockCancel()
	clock.Advance(500 * time.Millisecond)

	s.T().Log("third wait send to 2 destinations")
	thirdWr.Timeseries[0].Labels = append(thirdWr.Timeseries[0].Labels, prompb.Label{Name: "lname", Value: "blabla"})
	thirdWr.Timeseries = thirdWr.Timeseries[:1]
	thirdWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	secondWr.Timeseries[0].Samples[0].Value = math.NaN()
	secondWr.Timeseries[0].Samples[0].Timestamp = state.DefTimestamp()
	secondWr.Timeseries = append(secondWr.Timeseries, thirdWr.Timeseries...)
	s.Equal(secondWr.String(), <-destination)
	s.Equal(secondWr.String(), <-destination)

	s.T().Log("shutdown manager")
	shutdownCtx, cancel := context.WithTimeout(s.baseCtx, 1000*time.Millisecond)
	err = dstrb.Shutdown(shutdownCtx)
	cancel()
	s.Require().NoError(err)
}
