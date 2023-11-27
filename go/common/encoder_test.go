package common_test

import (
	"context"
	"math"
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/common"
)

type EncoderSuite struct {
	suite.Suite

	enc         *common.Encoder
	ctx         context.Context
	shardID     uint16
	encodeCount int
	bufSeg      []common.Segment
	expSegment  [][]byte
}

func TestEncoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderSuite))
}

func (s *EncoderSuite) SetupTest() {
	s.shardID = 0
	s.ctx = context.Background()
	s.enc = common.NewEncoder(s.shardID, 1)
	s.encodeCount = 100
	s.bufSeg = make([]common.Segment, 0, s.encodeCount)
	s.expSegment = make([][]byte, 0, s.encodeCount)
}

func (s *EncoderSuite) TearDownTest() {
	s.enc.Destroy()
}

func (s *EncoderSuite) makeData() []byte {
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
		},
	}

	b, err := wr.Marshal()
	s.NoError(err)
	return b
}

func (s *EncoderSuite) makeDataWithTwoTimeseries() []byte {
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
	s.NoError(err)
	return b
}

func (s *EncoderSuite) TestEncode() {
	s.T().Log("encode data and accumulate segment and redundant")
	hlimits := common.DefaultHashdexLimits()

	for i := 0; i < s.encodeCount; i++ {
		data := s.makeData()
		h, err := common.NewHashdex(data, hlimits)
		s.NoError(err)

		segKey, gos, err := s.enc.Encode(s.ctx, h)
		s.NoError(err)

		s.Equal(segKey.Segment, s.enc.LastEncodedSegment())
		s.bufSeg = append(s.bufSeg, gos)
		s.expSegment = append(s.expSegment, data)
	}
}

func (s *EncoderSuite) TestEncodeError() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	hlimits := common.DefaultHashdexLimits()

	h, err := common.NewHashdex(s.makeData(), hlimits)
	s.NoError(err)

	_, _, err2 := s.enc.Encode(ctx, h)
	s.Error(err2)
}

func (s *EncoderSuite) TestEncodeErrorCPPExceptions() {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
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
	s.Require().NoError(err)

	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(b, hlimits)
	s.Require().NoError(err)

	_, _, err = s.enc.Encode(s.ctx, h)
	s.Require().Error(err)
	s.True(
		common.IsExceptionCodeFromErrorAnyOf(err, 0x546e143d302c4860),
		"Exception code is %x: %+v",
		common.GetExceptionCodeFromError(err),
		err,
	)
}

func (s *EncoderSuite) TestFinalizeErrorCPPExceptions() {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
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
	s.Require().NoError(err)

	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(b, hlimits)
	s.Require().NoError(err)

	_, err = s.enc.Add(s.ctx, h)
	s.Require().NoError(err)

	_, _, err = s.enc.Finalize(s.ctx)
	s.Require().Error(err)
}

func (s *EncoderSuite) TestCppInvalidDataForHashdex() {
	invalidPbData := []byte("1111")
	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(invalidPbData, hlimits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

// Test encode remaining table size.
//

func (s *EncoderSuite) TestEncodeRemainingSize() {
	var remainingTableSize uint32 = math.MaxUint32
	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(s.makeData(), hlimits)
	s.NoError(err)
	seg, err := s.enc.Add(s.ctx, h)
	s.NoError(err)

	var prevRemainingTableSize = seg.RemainingTableSize()
	s.Less(prevRemainingTableSize, remainingTableSize)
}

//
// Test shards for memory limits
//

func (s *EncoderSuite) TestHashdexWithHardLimitsOnPbMessage() {
	limits := common.HashdexLimits{
		MaxPbSizeInBytes: 20,
	}
	data := s.makeData()
	h, err := common.NewHashdex(data, limits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *EncoderSuite) TestHashdexWithHardLimitsOnLabelNameLength() {
	limits := common.HashdexLimits{
		MaxLabelNameLength: 2,
	}
	data := s.makeData()
	h, err := common.NewHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *EncoderSuite) TestHashdexWithHardLimitsOnLabelValueLength() {
	limits := common.HashdexLimits{
		MaxLabelValueLength: 2,
	}
	data := s.makeData()
	h, err := common.NewHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *EncoderSuite) TestHashdexWithHardLimitsOnLabelsInTimeseries() {
	limits := common.HashdexLimits{
		MaxLabelNamesPerTimeseries: 1,
	}
	data := s.makeData()
	h, err := common.NewHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *EncoderSuite) TestHashdexWithVeryHardLimitsOnTimeseries() {
	limits := common.HashdexLimits{
		MaxTimeseriesCount: 1,
	}
	data := s.makeDataWithTwoTimeseries()
	h, err := common.NewHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

//
// Benchmarks

func BenchmarkEncoder(b *testing.B) {
	ctx := context.Background()
	enc := common.NewEncoder(0, 1)

	defer enc.Destroy()

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
		},
	}

	data, err := wr.Marshal()
	require.NoError(b, err)
	hlimits := common.DefaultHashdexLimits()

	for i := 0; i < b.N; i++ {
		h, _ := common.NewHashdex(data, hlimits)
		id, gos, err := enc.Encode(ctx, h)
		_, _, _ = id, gos, err
	}
}
