package cppbridge_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
)

type EncoderDecoderSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestEncoderDecoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderDecoderSuite))
}

func (s *EncoderDecoderSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (*EncoderDecoderSuite) makeData(count int, sid int64) *prompb.WriteRequest {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{},
	}

	var (
		startTime int64 = 1654608400000
		step      int64 = 60000
	)

	startTime += step * (sid * 3)

	for i := 0; i < count; i++ {
		wr.Timeseries = append(
			wr.Timeseries,
			prompb.TimeSeries{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test" + strconv.Itoa(i),
					},
					{
						Name:  "instance",
						Value: "blablabla" + strconv.Itoa(i),
					},
					{
						Name:  "job",
						Value: "tester" + strconv.Itoa(i),
					},
					{
						Name:  "low",
						Value: "banan" + strconv.Itoa(i),
					},
					{
						Name:  "zero",
						Value: "non_zero" + strconv.Itoa(i),
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: startTime,
						Value:     4444,
					},
					{
						Timestamp: startTime + step,
						Value:     4447,
					},
					{
						Timestamp: startTime + step*2,
						Value:     4448,
					},
				},
			},
		)
	}

	return wr
}

func (*EncoderDecoderSuite) transferringData(income frames.WritePayload) []byte {
	buf, _ := framestest.ReadPayload(income)
	return buf
}

func (s *EncoderDecoderSuite) TestEncodeDecode() {
	hlimits := cppbridge.DefaultHashdexLimits()
	dec := cppbridge.NewWALDecoder()
	enc := cppbridge.NewWALEncoder(0, 0)
	count := 10
	k := count + 1
	for i := 0; i < count; i++ {
		s.T().Log("generate protobuf")
		k--
		seriesCount := k * (i + 1)
		expectedWr := s.makeData(seriesCount, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		createdAt := time.Now()
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(seriesCount, gos.Series())
		s.EqualValues(seriesCount*3, gos.Samples())

		s.T().Log("transferring segment")
		segByte := s.transferringData(gos)

		s.T().Log("decoding protobuf")
		protocont, err := dec.Decode(s.baseCtx, segByte)
		s.Require().NoError(err)

		s.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		s.Require().NoError(protocont.UnmarshalTo(actualWr))
		s.Equal(expectedWr.String(), actualWr.String())

		s.InDelta(createdAt.UnixNano(), protocont.CreatedAt(), float64(time.Second))

		s.Equal(gos.Samples(), protocont.Samples())
		s.Equal(gos.Series(), protocont.Series())
	}
}

func (s *EncoderDecoderSuite) TestEncodeDecodeOpenHead() {
	createdAt := time.Now()
	hlimits := cppbridge.DefaultHashdexLimits()
	dec := cppbridge.NewWALDecoder()
	enc := cppbridge.NewWALEncoder(0, 0)
	for i := 1; i < 21; i++ {
		s.T().Log("generate protobuf")
		expectedWr := s.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		gosF, err := enc.Add(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(10, gosF.Series())
		s.EqualValues(30*i, gosF.Samples())
	}

	_, gos, err := enc.Finalize(s.baseCtx)
	s.Require().NoError(err)

	s.T().Log("transferring segment")
	segByte := s.transferringData(gos)

	s.T().Log("decoding protobuf")
	protob, err := dec.Decode(s.baseCtx, segByte)
	s.Require().NoError(err)

	s.Equal(gos.Samples(), protob.Samples())
	s.Equal(gos.Series(), protob.Series())

	s.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	s.Require().NoError(protob.UnmarshalTo(actualWr))

	s.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))
}

func (s *EncoderDecoderSuite) TestRestoreFromStream() {
	hlimits := cppbridge.DefaultHashdexLimits()
	enc := cppbridge.NewWALEncoder(0, 0)
	buf := make([]byte, 0)
	count := 10
	offsets := make([]uint64, count)

	s.T().Log("generate wal with segments")
	for i := 0; i < count; i++ {
		expectedWr := s.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)
		h, err := cppbridge.NewWALHashdex(data, hlimits)
		s.Require().NoError(err)
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)
		segByte := s.transferringData(gos)
		buf = append(buf, segByte...)
		offsets[i] = uint64(len(buf))
	}

	for i := range offsets {
		s.T().Logf("restore decoder for segment id: %d\n", i)
		dec := cppbridge.NewWALDecoder()
		offset, retoreSID, err := dec.RestoreFromStream(s.baseCtx, buf, uint32(i))
		s.Require().NoError(err)

		s.Equal(offsets[i], offset)
		s.Equal(uint32(i), retoreSID)
	}

	s.T().Logf("restore decoder for over segment id: %d\n", count)
	dec := cppbridge.NewWALDecoder()
	offset, retoreSID, err := dec.RestoreFromStream(s.baseCtx, buf, uint32(count))
	s.Require().NoError(err)

	s.Equal(uint64(len(buf)), offset)
	s.Equal(uint32(count-1), retoreSID)
}

// this test run for local  benchmark test
func (s *EncoderDecoderSuite) EncodeDecodeBench(i int64) {
	expectedWr := s.makeData(100, i)
	data, err := expectedWr.Marshal()
	s.Require().NoError(err)
	hlimits := cppbridge.DefaultHashdexLimits()
	dec := cppbridge.NewWALDecoder()
	enc := cppbridge.NewWALEncoder(0, 0)
	h, err := cppbridge.NewWALHashdex(data, hlimits)
	s.Require().NoError(err)

	_, gos, err := enc.Encode(s.baseCtx, h)
	s.Require().NoError(err)

	segByte := s.transferringData(gos)

	protob, err := dec.Decode(s.baseCtx, segByte)
	s.Require().NoError(err)
	_ = protob
}

// this test run for local  benchmark test
func (s *EncoderDecoderSuite) TestEncodeDecodeBenchmark() {
	for i := 0; i < 1000; i++ {
		s.EncodeDecodeBench(int64(i))
	}
}
