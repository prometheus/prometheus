package common_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
)

type EncoderDecoderSuite struct {
	suite.Suite

	ctx context.Context
	enc *common.Encoder
	dec *common.Decoder
}

func TestEncoderDecoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderDecoderSuite))
}

func (eds *EncoderDecoderSuite) SetupTest() {
	var err error
	eds.ctx = context.Background()
	eds.enc = common.NewEncoder(0, 1)
	eds.dec, err = common.NewDecoder()
	eds.NoError(err)
}

func (eds *EncoderDecoderSuite) TearDownTest() {
	eds.enc.Destroy()
	eds.dec.Destroy()
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

func (eds *EncoderDecoderSuite) TestEncodeDecode() {
	hlimits := common.DefaultHashdexLimits()
	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)

		eds.T().Log("encoding protobuf")
		createdAt := time.Now()
		_, gos, _, err := eds.enc.Encode(eds.ctx, h)
		eds.Require().NoError(err)

		eds.EqualValues(10, gos.Series())
		eds.EqualValues(30, gos.Samples())

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos)

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		eds.Require().NoError(protob.UnmarshalTo(actualWr))
		eds.Equal(expectedWr.String(), actualWr.String())

		eds.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeSnapshot() {
	rts := make([]common.Redundant, 5)
	segmentsBuffer := make([][]byte, 5)
	hlimits := common.DefaultHashdexLimits()

	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)

		eds.T().Log("encoding protobuf")
		_, gos, rt, err := eds.enc.Encode(eds.ctx, h)
		eds.Require().NoError(err)

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos)

		if i >= 5 {
			rts[i-5] = rt
			segmentsBuffer[i-5] = segByte
			continue
		}

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		if eds.NoError(protob.UnmarshalTo(actualWr)) {
			eds.Equal(expectedWr.String(), actualWr.String())
		}
	}

	eds.T().Log("get snapshot")
	gsnapshot, err := eds.enc.Snapshot(eds.ctx, rts)
	eds.Require().NoError(err)

	eds.T().Log("init new decoder")
	resDec, err := common.NewDecoder()
	eds.Require().NoError(err)

	eds.T().Log("restore new decoder")
	buf, err := framestest.ReadPayload(gsnapshot)
	eds.Require().NoError(err)
	err = resDec.Snapshot(eds.ctx, buf)
	eds.Require().NoError(err)

	eds.T().Log("send buffer segments")
	for _, segByte := range segmentsBuffer {
		eds.T().Log("after restore decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("after restore restored decoding protobuf")
		resProtob, _, err := resDec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("after restore compare income and outcome and restored protobuf")
		actualWr := &prompb.WriteRequest{}
		eds.NoError(protob.UnmarshalTo(actualWr))

		resActualWr := &prompb.WriteRequest{}
		eds.NoError(resProtob.UnmarshalTo(resActualWr))

		eds.Equal(actualWr.String(), resActualWr.String())
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeSnapshotWithDrySegment() {
	rts := make([]common.Redundant, 5)
	segmentsDry := make([][]byte, 5)
	hlimits := common.DefaultHashdexLimits()

	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)

		eds.T().Log("encoding protobuf")
		_, gos, rt, err := eds.enc.Encode(eds.ctx, h)
		eds.Require().NoError(err)

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos)

		if i >= 5 {
			rts[i-5] = rt
			segmentsDry[i-5] = segByte
		}

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		eds.Require().NoError(protob.UnmarshalTo(actualWr))
		eds.Equal(expectedWr.String(), actualWr.String())
	}

	eds.T().Log("get snapshot")
	gsnapshot, err := eds.enc.Snapshot(eds.ctx, rts)
	eds.Require().NoError(err)

	eds.T().Log("init new decoder")
	resDec, err := common.NewDecoder()
	eds.Require().NoError(err)

	eds.T().Log("restore new decoder")
	buf, err := framestest.ReadPayload(gsnapshot)
	eds.Require().NoError(err)
	err = resDec.Snapshot(eds.ctx, buf)
	eds.Require().NoError(err)

	eds.T().Log("restore segmentsDry")
	for i := range segmentsDry {
		err = resDec.DecodeDry(eds.ctx, segmentsDry[i])
		eds.Require().NoError(err)
	}

	eds.T().Log("after restore generate protobuf")
	expectedWr := eds.makeData(10, 10)
	data, err := expectedWr.Marshal()
	eds.Require().NoError(err)

	eds.T().Log("after restore sharding protobuf")
	h, err := common.NewHashdex(data, hlimits)
	eds.Require().NoError(err)

	eds.T().Log("after restore encoding protobuf")
	_, gos, _, err := eds.enc.Encode(eds.ctx, h)
	eds.Require().NoError(err)

	eds.T().Log("after restore transferring segment")
	segByte := eds.transferringData(gos)

	eds.T().Log("after restore decoding protobuf")
	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("after restore restored decoding protobuf")
	resProtob, _, err := resDec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("after restore compare income and outcome and restored protobuf")
	actualWr := &prompb.WriteRequest{}
	eds.Require().NoError(protob.UnmarshalTo(actualWr))
	eds.Equal(expectedWr.String(), actualWr.String())

	resActualWr := &prompb.WriteRequest{}
	eds.Require().NoError(resProtob.UnmarshalTo(resActualWr))
	eds.Equal(expectedWr.String(), resActualWr.String())
}

// this test run for local  benchmark test
func (eds *EncoderDecoderSuite) EncodeDecodeBench(i int64) {
	expectedWr := eds.makeData(100, i)
	data, err := expectedWr.Marshal()
	eds.Require().NoError(err)
	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(data, hlimits)
	eds.Require().NoError(err)

	_, gos, _, err := eds.enc.Encode(eds.ctx, h)
	eds.Require().NoError(err)

	segByte := eds.transferringData(gos)

	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)
	_ = protob
}

// this test run for local  benchmark test
func (eds *EncoderDecoderSuite) TestEncodeDecodeBenchmark() {
	for i := 0; i < 1000; i++ {
		eds.EncodeDecodeBench(int64(i))
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeOpenHead() {
	createdAt := time.Now()
	hlimits := common.DefaultHashdexLimits()
	for i := 1; i < 21; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)

		eds.T().Log("encoding protobuf")
		gosF, err := eds.enc.Add(eds.ctx, h)
		eds.Require().NoError(err)

		eds.EqualValues(10, gosF.Series())
		eds.EqualValues(30*i, gosF.Samples())
	}

	_, gos, _, err := eds.enc.Finalize(eds.ctx)
	eds.Require().NoError(err)

	eds.T().Log("transferring segment")
	segByte := eds.transferringData(gos)

	eds.T().Log("Series", gos.Series())
	eds.T().Log("Samples", gos.Samples())

	eds.T().Log("decoding protobuf")
	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	eds.Require().NoError(protob.UnmarshalTo(actualWr))
	eds.T().Log("SeriesB", len(actualWr.Timeseries))

	eds.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))
}
