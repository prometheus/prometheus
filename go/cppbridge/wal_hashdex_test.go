package cppbridge_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HashdexSuite struct {
	suite.Suite
	baseCtx        context.Context
	startTimestamp int64
	step           int64
}

func TestHashdexSuite(t *testing.T) {
	suite.Run(t, new(HashdexSuite))
}

func (s *HashdexSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.startTimestamp = 1654608420000
	s.step = 60000
}

func (s *HashdexSuite) makeData(i int64) []byte {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: s.startTimestamp + (s.step * i),
						Value:     4444,
					},
					{
						Timestamp: s.startTimestamp + (s.step * i * 2),
						Value:     4445,
					},
				},
			},
		},
	}

	b, err := wr.Marshal()
	s.Require().NoError(err)
	return b
}

func (s *HashdexSuite) makeDataWithTwoTimeseries() []byte {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420000,
						Value:     4444,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420666,
						Value:     4666,
					},
				},
			},
		},
	}

	b, err := wr.Marshal()
	s.Require().NoError(err)
	return b
}

func (s *HashdexSuite) TestCppInvalidDataForHashdex() {
	invalidPbData := []byte("1111")
	hlimits := cppbridge.DefaultHashdexLimits()
	h, err := cppbridge.NewWALHashdex(invalidPbData, hlimits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnPbMessage() {
	limits := cppbridge.HashdexLimits{
		MaxPbSizeInBytes: 20,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALHashdex(data, limits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelNameLength() {
	limits := cppbridge.HashdexLimits{
		MaxLabelNameLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelValueLength() {
	limits := cppbridge.HashdexLimits{
		MaxLabelValueLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelsInTimeseries() {
	limits := cppbridge.HashdexLimits{
		MaxLabelNamesPerTimeseries: 1,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithVeryHardLimitsOnTimeseries() {
	limits := cppbridge.HashdexLimits{
		MaxTimeseriesCount: 1,
	}
	data := s.makeDataWithTwoTimeseries()
	h, err := cppbridge.NewWALHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func TestHashdex_ClusterReplica(t *testing.T) {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "cluster",
						Value: "cluster-0",
					},
					{
						Name:  "__replica__",
						Value: "replica-1",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: -1654608420000,
						Value:     4444,
					},
				},
			},
		},
	}
	b, err := wr.Marshal()
	require.NoError(t, err)

	hlimits := cppbridge.DefaultHashdexLimits()
	h, err := cppbridge.NewWALHashdex(b, hlimits)
	require.NoError(t, err)
	cluster := h.Cluster()
	replica := h.Replica()
	t.Log("clean source protobuf message")
	copy(b, make([]byte, len(b)))
	t.Log("check that cluster and replica stay unchanged")
	assert.Equal(t, "cluster-0", cluster)
	assert.Equal(t, "replica-1", replica)
}
