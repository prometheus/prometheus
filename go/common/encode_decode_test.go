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
	count := 10
	k := count + 1
	for i := 0; i < count; i++ {
		eds.T().Log("generate protobuf")
		k--
		seriesCount := k * (i + 1)
		expectedWr := eds.makeData(seriesCount, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)

		eds.T().Log("sharding protobuf")
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)

		eds.T().Log("encoding protobuf")
		createdAt := time.Now()
		_, gos, err := eds.enc.Encode(eds.ctx, h)
		eds.Require().NoError(err)

		eds.EqualValues(seriesCount, gos.Series())
		eds.EqualValues(seriesCount*3, gos.Samples())

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

		eds.Equal(gos.Samples(), protob.Samples())
		eds.Equal(gos.Series(), protob.Series())
	}
}

// this test run for local  benchmark test
func (eds *EncoderDecoderSuite) EncodeDecodeBench(i int64) {
	expectedWr := eds.makeData(100, i)
	data, err := expectedWr.Marshal()
	eds.Require().NoError(err)
	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(data, hlimits)
	eds.Require().NoError(err)

	_, gos, err := eds.enc.Encode(eds.ctx, h)
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

	_, gos, err := eds.enc.Finalize(eds.ctx)
	eds.Require().NoError(err)

	eds.T().Log("transferring segment")
	segByte := eds.transferringData(gos)

	eds.T().Log("Series", gos.Series())
	eds.T().Log("Samples", gos.Samples())

	eds.T().Log("decoding protobuf")
	protob, _, err := eds.dec.Decode(eds.ctx, segByte)
	eds.Require().NoError(err)

	eds.Equal(gos.Samples(), protob.Samples())
	eds.Equal(gos.Series(), protob.Series())

	eds.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	eds.Require().NoError(protob.UnmarshalTo(actualWr))
	eds.T().Log("SeriesB", len(actualWr.Timeseries))

	eds.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))
}

func (eds *EncoderDecoderSuite) TestRestoreFromStream() {
	hlimits := common.DefaultHashdexLimits()
	buf := make([]byte, 0)
	count := 10
	offsets := make([]uint64, count)

	eds.T().Log("generate wal with segments")
	for i := 0; i < count; i++ {
		expectedWr := eds.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		eds.Require().NoError(err)
		h, err := common.NewHashdex(data, hlimits)
		eds.Require().NoError(err)
		_, gos, err := eds.enc.Encode(eds.ctx, h)
		eds.Require().NoError(err)
		segByte := eds.transferringData(gos)
		buf = append(buf, segByte...)
		offsets[i] = uint64(len(buf))
	}

	for i := range offsets {
		eds.T().Logf("restore decoder for segment id: %d\n", i)
		dec, err := common.NewDecoder()
		eds.Require().NoError(err)
		offset, retoreSID, err := dec.RestoreFromStream(eds.ctx, buf, uint32(i))
		eds.Require().NoError(err)
		dec.Destroy()

		eds.Equal(offsets[i], offset)
		eds.Equal(uint32(i), retoreSID)
	}

	eds.T().Logf("restore decoder for over segment id: %d\n", count)
	dec, err := common.NewDecoder()
	eds.Require().NoError(err)
	offset, retoreSID, err := dec.RestoreFromStream(eds.ctx, buf, uint32(count))
	eds.Require().NoError(err)
	dec.Destroy()

	eds.Equal(uint64(len(buf)), offset)
	eds.Equal(uint32(count-1), retoreSID)
}
