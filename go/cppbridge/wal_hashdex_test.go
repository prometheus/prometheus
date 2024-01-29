package cppbridge_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/go-faker/faker/v4/pkg/options"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/pp/go/model"
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
	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(invalidPbData, hlimits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelNameLength() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelNameLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelValueLength() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelValueLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelsInTimeseries() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelNamesPerTimeseries: 1,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithVeryHardLimitsOnTimeseries() {
	limits := cppbridge.WALHashdexLimits{
		MaxTimeseriesCount: 1,
	}
	data := s.makeDataWithTwoTimeseries()
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

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

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(b, hlimits)
	require.NoError(t, err)
	cluster := h.Cluster()
	replica := h.Replica()
	t.Log("clean source protobuf message")
	copy(b, make([]byte, len(b)))
	t.Log("check that cluster and replica stay unchanged")
	assert.Equal(t, "cluster-0", cluster)
	assert.Equal(t, "replica-1", replica)
}

const (
	clusterLabelName = "cluster"
	replicaLabelName = "__replica__"
)

type GoModelHashdexTestSuite struct {
	suite.Suite
	ctx  context.Context
	rand *rand.Rand
}

func TestGoModelHashdexTestSuite(t *testing.T) {
	suite.Run(t, new(GoModelHashdexTestSuite))
}

func (s *GoModelHashdexTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func (s *GoModelHashdexTestSuite) makeTestData(limits cppbridge.WALHashdexLimits, size int) []model.TimeSeries {
	tss := make([]model.TimeSeries, 0, size)

	for i := 0; i < size; i++ {
		lsb := model.NewLabelSetBuilder()
		for j := 0; j < 1+s.rand.Intn(int(limits.MaxLabelNamesPerTimeseries-1)); j++ {
			lsb.Set(
				faker.Word(options.WithRandomStringLength(1+uint(s.rand.Intn(int(limits.MaxLabelNameLength-1))))),
				faker.Word(options.WithRandomStringLength(1+uint(s.rand.Intn(int(limits.MaxLabelValueLength-1))))),
			)
		}
		ts := model.TimeSeries{
			LabelSet:  lsb.Build(),
			Timestamp: uint64(time.Now().Unix()),
			Value:     s.rand.ExpFloat64(),
		}
		tss = append(tss, ts)
	}
	return tss
}

func (s *GoModelHashdexTestSuite) TestHappyPath() {
	limits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder()
	enc := cppbridge.NewWALEncoder(0, 0)

	testData := s.makeTestData(limits, 1000)
	hdx, err := cppbridge.NewWALGoModelHashdex(limits, testData)
	s.Require().NoError(err)

	segStat, err := enc.Add(s.ctx, hdx)
	s.Require().NoError(err)

	s.Require().EqualValues(len(testData), segStat.Series())

	_, seg, err := enc.Finalize(s.ctx)
	s.Require().NoError(err)

	buf, err := framestest.ReadPayload(seg)
	s.Require().NoError(err)

	protoData, err := dec.Decode(s.ctx, buf)
	s.Require().NoError(err)

	prometheusWriteRequest := &prompb.WriteRequest{}
	s.Require().NoError(protoData.UnmarshalTo(prometheusWriteRequest))

	s.Require().EqualValues(len(testData), len(prometheusWriteRequest.Timeseries))

	for i := 0; i < len(testData); i++ {
		s.Require().Len(prometheusWriteRequest.Timeseries[i].Samples, 1)
		s.Require().EqualValues(testData[i].Timestamp, prometheusWriteRequest.Timeseries[i].Samples[0].Timestamp)
		s.Require().EqualValues(testData[i].Value, prometheusWriteRequest.Timeseries[i].Samples[0].Value)
		for j := 0; j < testData[i].LabelSet.Len(); j++ {
			expectedName := testData[i].LabelSet.Key(j)
			expectedValue := testData[i].LabelSet.Value(j)
			s.Require().Equal(expectedName, prometheusWriteRequest.Timeseries[i].Labels[j].Name)
			s.Require().Equal(expectedValue, prometheusWriteRequest.Timeseries[i].Labels[j].Value)
		}
	}
}

func (s *GoModelHashdexTestSuite) TestHashdexLabels() {
	cluster := "the_cluster"
	replica := "the_replica"

	// cluster & replica labels
	{
		limits := cppbridge.DefaultWALHashdexLimits()
		ts := model.TimeSeries{
			LabelSet: model.NewLabelSetBuilder().
				Set(clusterLabelName, cluster).
				Set(replicaLabelName, replica).
				Build(),
			Timestamp: 42,
			Value:     25,
		}

		hdx, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().NoError(err)
		require.Equal(s.T(), cluster, hdx.Cluster())
		require.Equal(s.T(), replica, hdx.Replica())
	}

	// max label name size exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxLabelNameLength: 5,
		}

		ts := model.TimeSeries{
			LabelSet:  model.NewLabelSetBuilder().Set("123456", "value").Build(),
			Timestamp: 42,
			Value:     25,
		}
		_, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().Error(err)
	}

	// max label value size exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxLabelValueLength: 5,
		}

		ts := model.TimeSeries{
			LabelSet:  model.NewLabelSetBuilder().Set("123456", "1234567").Build(),
			Timestamp: 42,
			Value:     25,
		}
		_, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().Error(err)
	}

	// max number of time series exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxTimeseriesCount: 1,
		}

		tss := []model.TimeSeries{
			{}, {},
		}

		_, err := cppbridge.NewWALGoModelHashdex(limits, tss)
		s.Require().Error(err)
	}
}
