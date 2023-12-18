package cppbridge_test

import (
	"context"
	"math"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
)

type EncoderSuite struct {
	suite.Suite
	baseCtx        context.Context
	startTimestamp int64
	step           int64
}

func TestEncoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderSuite))
}

func (s *EncoderSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.startTimestamp = 1654608420000
	s.step = 60000
}

func (s *EncoderSuite) makeData(i int64) []byte {
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

func (*EncoderSuite) transferringData(income frames.WritePayload) []byte {
	buf, _ := framestest.ReadPayload(income)
	return buf
}

func (s *EncoderSuite) TestEncode() {
	s.T().Log("encode data")
	hlimits := cppbridge.DefaultHashdexLimits()
	encodeCount := 10000
	enc := cppbridge.NewWALEncoder(0, 0)

	for i := 0; i < encodeCount; i++ {
		h, err := cppbridge.NewWALHashdex(s.makeData(int64(i)), hlimits)
		s.Require().NoError(err)

		segKey, seg, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)
		s.Equal(segKey.Segment, enc.LastEncodedSegment())

		s.Equal(s.startTimestamp+(s.step*int64(i)), seg.EarliestTimestamp())
		s.Equal(s.startTimestamp+(s.step*int64(i)*2), seg.LatestTimestamp())
		s.EqualValues(4294967276, seg.RemainingTableSize())
		s.EqualValues(1, seg.Series())
		s.EqualValues(2, seg.Samples())
		size := seg.Size()

		tbyte := s.transferringData(seg)
		s.EqualValues(size, len(tbyte))
	}
}

func (s *EncoderSuite) TestEncodeError() {
	ctx, cancel := context.WithCancel(s.baseCtx)
	cancel()
	hlimits := cppbridge.DefaultHashdexLimits()

	h, err := cppbridge.NewWALHashdex(s.makeData(int64(1)), hlimits)
	s.Require().NoError(err)

	enc := cppbridge.NewWALEncoder(0, 0)
	_, _, err2 := enc.Encode(ctx, h)
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

	hlimits := cppbridge.DefaultHashdexLimits()
	h, err := cppbridge.NewWALHashdex(b, hlimits)
	s.Require().NoError(err)

	enc := cppbridge.NewWALEncoder(0, 0)
	_, _, err = enc.Encode(s.baseCtx, h)
	s.Require().Error(err)
	s.True(
		cppbridge.IsExceptionCodeFromErrorAnyOf(err, 0x546e143d302c4860),
		"Exception code is %x: %+v",
		cppbridge.GetExceptionCodeFromError(err),
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

	hlimits := cppbridge.DefaultHashdexLimits()
	h, err := cppbridge.NewWALHashdex(b, hlimits)
	s.Require().NoError(err)

	enc := cppbridge.NewWALEncoder(0, 0)
	_, err = enc.Add(s.baseCtx, h)
	s.Require().NoError(err)

	_, _, err = enc.Finalize(s.baseCtx)
	s.Require().Error(err)
}

func (s *EncoderSuite) TestEncodeRemainingSize() {
	var remainingTableSize uint32 = math.MaxUint32
	hlimits := cppbridge.DefaultHashdexLimits()
	h, err := cppbridge.NewWALHashdex(s.makeData(1), hlimits)
	s.NoError(err)

	enc := cppbridge.NewWALEncoder(0, 0)
	seg, err := enc.Add(s.baseCtx, h)
	s.NoError(err)

	var prevRemainingTableSize = seg.RemainingTableSize()
	s.Less(prevRemainingTableSize, remainingTableSize)
}
