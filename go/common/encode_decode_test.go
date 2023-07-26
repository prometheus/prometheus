package common_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/common"
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
	eds.ctx = context.Background()
	eds.enc = common.NewEncoder(0, 1)
	eds.dec = common.NewDecoder()
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

func (*EncoderDecoderSuite) transferringData(income []byte) []byte {
	outcome := make([]byte, len(income))
	copy(outcome, income)
	return outcome
}

func (eds *EncoderDecoderSuite) TestEncodeDecode() {
	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h := common.NewHashdex(data)

		eds.T().Log("encoding protobuf")
		createdAt := time.Now()
		_, gos, gor, err := eds.enc.Encode(eds.ctx, h)
		eds.T().Log("destroy hashdex")
		h.Destroy()
		eds.Require().NoError(err)

		eds.EqualValues(10, gos.Series())
		eds.EqualValues(30, gos.Samples())

		eds.T().Log("destroy redundant")
		gor.Destroy()

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos.Bytes())

		eds.T().Log("destroy encoding segment")
		gos.Destroy()

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		err = actualWr.Unmarshal(protob.Bytes())
		eds.Require().NoError(err)
		eds.Equal(expectedWr.String(), actualWr.String())

		eds.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))

		eds.T().Log("destroy decoding proto")
		protob.Destroy()
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeSnapshot() {
	rts := make([]common.Redundant, 5)
	segmentsBuffer := make([][]byte, 5)

	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h := common.NewHashdex(data)

		eds.T().Log("encoding protobuf")
		_, gos, rt, err := eds.enc.Encode(eds.ctx, h)
		eds.T().Log("destroy hashdex")
		h.Destroy()
		eds.Require().NoError(err)

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos.Bytes())

		eds.T().Log("destroy encoding segment")
		gos.Destroy()

		if i >= 5 {
			rts[i-5] = rt
			segmentsBuffer[i-5] = segByte
			continue
		}
		rt.Destroy()

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		err = actualWr.Unmarshal(protob.Bytes())
		eds.Require().NoError(err)
		eds.Equal(expectedWr.String(), actualWr.String())

		eds.T().Log("destroy decoding proto")
		protob.Destroy()
	}

	eds.T().Log("get snapshot")
	gsnapshot, err := eds.enc.Snapshot(eds.ctx, rts)
	eds.Require().NoError(err)

	eds.T().Log("destroy redundants")
	for i := range rts {
		rts[i].Destroy()
	}

	eds.T().Log("init new decoder")
	resDec := common.NewDecoder()

	eds.T().Log("restore new decoder")
	err = resDec.Snapshot(eds.ctx, gsnapshot.Bytes())
	eds.Require().NoError(err)
	gsnapshot.Destroy()

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
		err = actualWr.Unmarshal(protob.Bytes())
		eds.Require().NoError(err)

		resActualWr := &prompb.WriteRequest{}
		err = resActualWr.Unmarshal(resProtob.Bytes())
		eds.Require().NoError(err)

		eds.Equal(actualWr.String(), resActualWr.String())

		eds.T().Log("after restore destroy decoding proto")
		protob.Destroy()
		resProtob.Destroy()
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeSnapshotWithDrySegment() {
	rts := make([]common.Redundant, 5)
	segmentsDry := make([][]byte, 5)

	for i := 0; i < 10; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h := common.NewHashdex(data)

		eds.T().Log("encoding protobuf")
		_, gos, rt, err := eds.enc.Encode(eds.ctx, h)
		eds.T().Log("destroy hashdex")
		h.Destroy()
		eds.Require().NoError(err)

		eds.T().Log("transferring segment")
		segByte := eds.transferringData(gos.Bytes())

		eds.T().Log("destroy encoding segment")
		gos.Destroy()

		if i >= 5 {
			rts[i-5] = rt
			segmentsDry[i-5] = segByte
		} else {
			rt.Destroy()
		}

		eds.T().Log("decoding protobuf")
		protob, _, err := eds.dec.Decode(eds.ctx, segByte)
		eds.Require().NoError(err)

		eds.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		err = actualWr.Unmarshal(protob.Bytes())
		eds.Require().NoError(err)
		eds.Equal(expectedWr.String(), actualWr.String())

		eds.T().Log("destroy decoding proto")
		protob.Destroy()
	}

	eds.T().Log("get snapshot")
	gsnapshot, err := eds.enc.Snapshot(eds.ctx, rts)
	eds.Require().NoError(err)

	eds.T().Log("destroy redundants")
	for i := range rts {
		rts[i].Destroy()
	}

	eds.T().Log("init new decoder")
	resDec := common.NewDecoder()

	eds.T().Log("restore new decoder")
	err = resDec.Snapshot(eds.ctx, gsnapshot.Bytes())
	eds.Require().NoError(err)
	gsnapshot.Destroy()

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
	h := common.NewHashdex(data)

	eds.T().Log("after restore encoding protobuf")
	_, gos, rt, err := eds.enc.Encode(eds.ctx, h)
	eds.T().Log("after restore destroy hashdex")
	h.Destroy()
	eds.Require().NoError(err)
	rt.Destroy()

	eds.T().Log("after restore transferring segment")
	segByte := eds.transferringData(gos.Bytes())

	eds.T().Log("after restore destroy encoding segment")
	gos.Destroy()

	eds.T().Log("after restore decoding protobuf")
	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("after restore restored decoding protobuf")
	resProtob, _, err := resDec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("after restore compare income and outcome and restored protobuf")
	actualWr := &prompb.WriteRequest{}
	err = actualWr.Unmarshal(protob.Bytes())
	eds.Require().NoError(err)
	eds.Equal(expectedWr.String(), actualWr.String())

	resActualWr := &prompb.WriteRequest{}
	err = resActualWr.Unmarshal(resProtob.Bytes())
	eds.Require().NoError(err)
	eds.Equal(expectedWr.String(), resActualWr.String())

	eds.T().Log("after restore destroy decoding proto")
	protob.Destroy()
	resProtob.Destroy()
}

// this test run for local  benchmark test
func (eds *EncoderDecoderSuite) EncodeDecodeBench(i int64) {
	expectedWr := eds.makeData(100, i)
	data, err := expectedWr.Marshal()
	eds.Require().NoError(err)
	h := common.NewHashdex(data)
	_, gos, gor, err := eds.enc.Encode(eds.ctx, h)
	h.Destroy()
	gor.Destroy()
	eds.Require().NoError(err)

	segByte := eds.transferringData(gos.Bytes())
	gos.Destroy()

	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)
	protob.Destroy()
}

// this test run for local  benchmark test
func (eds *EncoderDecoderSuite) TestEncodeDecodeBenchmark() {
	for i := 0; i < 1000; i++ {
		eds.EncodeDecodeBench(int64(i))
	}
}

func (eds *EncoderDecoderSuite) TestEncodeDecodeOpenHead() {
	createdAt := time.Now()
	for i := 1; i < 21; i++ {
		eds.T().Log("generate protobuf")
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h := common.NewHashdex(data)

		eds.T().Log("encoding protobuf")
		gosF, err := eds.enc.Add(eds.ctx, h)
		eds.T().Log("destroy hashdex")
		h.Destroy()
		eds.Require().NoError(err)

		eds.EqualValues(10, gosF.Series())
		eds.EqualValues(30*i, gosF.Samples())
		gosF.Destroy()
	}

	_, gos, gor, err := eds.enc.Finalize(eds.ctx)
	eds.Require().NoError(err)

	eds.T().Log("destroy redundant")
	gor.Destroy()

	eds.T().Log("transferring segment")
	segByte := eds.transferringData(gos.Bytes())

	eds.T().Log("destroy encoding segment")
	eds.T().Log("Series", gos.Series())
	eds.T().Log("Samples", gos.Samples())
	gos.Destroy()

	eds.T().Log("decoding protobuf")
	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	err = actualWr.Unmarshal(protob.Bytes())
	eds.Require().NoError(err)
	eds.T().Log("SeriesB", len(actualWr.Timeseries))

	eds.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))

	eds.T().Log("destroy decoding proto")
	protob.Destroy()
}
